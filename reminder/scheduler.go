package reminder

import (
	"database/sql"
	"fmt"
	ts "nutribot/timestamp"
	"nutribot/types"
	"time"
	"nutribot/database"

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
    
    users, err := database.GetAllUsersWithReminders(s.db)
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