package reminder

import (
	"database/sql"
	"fmt"
	ts "nutribot/timestamp"
	"nutribot/types"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Scheduler struct {
	bot           *tgbotapi.BotAPI
	db            *sql.DB
	offsetManager *ts.OffsetManager
	stopChan      chan struct{}
}

func NewScheduler(bot *tgbotapi.BotAPI, db *sql.DB, offset *ts.OffsetManager) *Scheduler {
	return &Scheduler{
		bot:           bot,
		db:            db,
		offsetManager: offset,
		stopChan:      make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.checkReminders()
			case <-s.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
	fmt.Println("Планировщик напоминаний запущен...")
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
	fmt.Println("Планировщик напоминаний остановлен.")
}

func (s *Scheduler) checkReminders() {
	currentUTC := time.Now().UTC()

	users, err := s.getAllUsersWithReminders()
	if err != nil {
		fmt.Printf("Ошибка получения пользователей из БД: %v\n", err)
		return
	}
	
	for _, user := range users {
		if user.State < 0 {
			continue
		}
		
		userTime, err := s.getUserTime(user.Id, currentUTC)
		if err != nil {
			fmt.Printf("Ошибка получения времени для пользователя %d: %v\n", user.Id, err)
			continue
		}
		
		if s.shouldRemind(user, userTime) {
			s.sendRemind(user)
		}
	}
}

func (s *Scheduler) getAllUsersWithReminders() ([]types.User, error) {
	rows, err := s.db.Query(`
		SELECT id, telegram_id, chat_id, name, age, "offset", state 
		FROM users
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []types.User
	for rows.Next() {
		var user types.User
		var dbID int64
		err := rows.Scan(
			&dbID,
			&user.Id,
			&user.ChatID,
			&user.Name,
			&user.Age,
			&user.Offset,
			&user.State,
		)
		if err != nil {
			return nil, err
		}

		reminders, err := s.getRemindersByUserID(dbID)
		if err != nil {
			fmt.Printf("Ошибка получения напоминаний для пользователя %d: %v\n", user.Id, err)
			continue
		}
		user.Time = reminders
		
		users = append(users, user)
	}
	
	return users, rows.Err()
}

func (s *Scheduler) getRemindersByUserID(dbID int64) ([]types.TimeEntry, error) {
	rows, err := s.db.Query(`
		SELECT hour, minute FROM reminders 
		WHERE user_id = ? 
		ORDER BY hour, minute
	`, dbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []types.TimeEntry
	for rows.Next() {
		var r types.TimeEntry
		err := rows.Scan(&r.Hour, &r.Minute)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	
	return reminders, rows.Err()
}

func (s *Scheduler) getUserTime(id int, current time.Time) (time.Time, error) {
	offset, ok := s.offsetManager.GetUserOffset(int64(id))
	if !ok {
		return time.Time{}, fmt.Errorf("сдвиг не найден для пользователя %d", id)
	}

	loc := time.FixedZone("User", offset*3600)
	return current.In(loc), nil
}

func (s *Scheduler) shouldRemind(user types.User, uTime time.Time) bool {
	currentHour := uTime.Hour()
	currentMinute := uTime.Minute()
	
	for _, reminder := range user.Time {
		if reminder.Hour == currentHour && reminder.Minute == currentMinute {
			return true
		}
	}
	return false
}

func (s *Scheduler) sendRemind(user types.User) {
	msg := tgbotapi.NewMessage(int64(user.ChatID), fmt.Sprintf("%s, время принять пищу!", user.Name))
	_, err := s.bot.Send(msg)

	if err != nil {
		fmt.Printf("Ошибка отправки сообщения пользователю %d: %v\n", user.Id, err)
	}
}