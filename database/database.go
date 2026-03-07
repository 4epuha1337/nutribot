package database

import (
	"database/sql"
	"fmt"
	"log"
	"nutribot/types"
	ts "nutribot/timestamp"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./bot.db")
	if err != nil {
		return nil, err
	}
	
	if err = db.Ping(); err != nil {
		return nil, err
	}

	if err = createTables(db); err != nil {
		return nil, err
	}
	return db, nil
}

func createTables(db *sql.DB) error {
	userTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_id INTEGER UNIQUE NOT NULL,
		chat_id INTEGER NOT NULL,
		name TEXT,
		age INTEGER,
		offset INTEGER DEFAULT 0,
		state INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	reminderTable := `
	CREATE TABLE IF NOT EXISTS reminders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		hour INTEGER NOT NULL,
		minute INTEGER NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(user_id, hour, minute)
	);`
	indexUser := `CREATE INDEX IF NOT EXISTS idx_telegram_id ON users(telegram_id);`
	indexReminders := `CREATE INDEX IF NOT EXISTS idx_user_id ON reminders(user_id);`

	queries := []string{userTable, reminderTable, indexUser, indexReminders}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadOffsets(db *sql.DB, offsetManager *ts.OffsetManager) error {
    query := `SELECT telegram_id, "offset" FROM users`
    
    rows, err := db.Query(query)
    if err != nil {
        return fmt.Errorf("ошибка загрузки offset: %v", err)
    }
    defer rows.Close()
    
    loadedCount := 0
    for rows.Next() {
        var tgid int64
        var offset int
        
        if err := rows.Scan(&tgid, &offset); err != nil {
            log.Printf("Ошибка сканирования offset: %v", err)
            continue
        }
        
        if err := offsetManager.SetUserOffset(tgid, offset); err != nil {
            log.Printf("Ошибка установки offset для пользователя %d: %v", tgid, err)
        } else {
            loadedCount++
        }
    }
    
    if err = rows.Err(); err != nil {
        return fmt.Errorf("ошибка при итерации по строкам: %v", err)
    }
    
    log.Printf("Загружено %d смещений часовых поясов", loadedCount)
    return nil
}

func CreateUser(db *sql.DB, tgid int, chatid int, name string, age int, offset int, state int) (int64, error) {
	query := `INSERT INTO users (telegram_id, chat_id, name, age, "offset", state) VALUES (?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(query, tgid, chatid, name, age, offset, state)
	if err != nil {
		return -1, err
	}

	userID, err := res.LastInsertId()
	if err != nil {
    	return 0, err
	}

	return userID, nil
}

func GetUserByTelegramId(db *sql.DB, tgid int) (*types.User, int64, error) {
	query := `SELECT id, chat_id, name, age, "offset", state FROM users WHERE telegram_id = ?`
	var user types.User
	var id int64
	err := db.QueryRow(query, tgid).Scan(
		&id,
		&user.ChatID,
		&user.Name,
		&user.Age,
		&user.Offset,
		&user.State,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, -1, nil
		}
		return nil, -1, err
	}

	user.Id = tgid

	return &user, id, nil
}

func GetUserById(db *sql.DB, id int64) (*types.User, error) {
	query := `SELECT telegram_id, chat_id, name, age, "offset", state FROM users WHERE id = ?`
	var user types.User
	err := db.QueryRow(query, id).Scan(
		&user.Id,
		&user.ChatID,
		&user.Name,
		&user.Age,
		&user.Offset,
		&user.State,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func UpdateUserState(db *sql.DB, tgid int, state int) (error) {
	query := `UPDATE users SET state = ? WHERE telegram_id = ?`

	result, err := db.Exec(query, state, tgid)
	if err != nil {
		return fmt.Errorf("ошибка обновления состояния: %v", err)
	}

	rAff, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rAff == 0 {
		return fmt.Errorf("пользователь не найден")
	}

	return nil
}

func DeleteUser(db *sql.DB, tgid int) (error) {
	query := `DELETE FROM users WHERE telegram_id = ?`
	result, err := db.Exec(query, tgid)
	if err != nil {
		return fmt.Errorf("ошибка удаления пользователя: %v", err)
	}

	rAff, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rAff == 0 {
		return fmt.Errorf("пользователь не найден")
	}

	return nil
}

func UpdateUserProfile(db *sql.DB, tgid int, offset int, name string, age int) (error) {
	query := `UPDATE users SET "offset" = ?, name = ?, age = ? WHERE telegram_id = ?`

	result, err := db.Exec(query, offset, name, age, tgid)
	if err != nil {
		return fmt.Errorf("ошибка обновления профиля: %v", err)
	}

	rAff, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rAff == 0 {
		return fmt.Errorf("пользователь не найден")
	}

	return nil
}

func AddReminders(db *sql.DB, id int, reminders []types.TimeEntry) error {
	tx, err := db.Begin()

	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO reminders (user_id, hour, minute) VALUES (?, ?, ?)`
	for _, reminder := range reminders {
		_, err := tx.Exec(query, id, reminder.Hour, reminder.Minute)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetRemindersByUserId(db *sql.DB, id int64) ([]types.TimeEntry, error) {
	query := `SELECT hour, minute FROM reminders WHERE user_id = ? ORDER BY hour, minute`
	rows, err := db.Query(query, id)
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

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return reminders, nil
}

func GetFullUserByTelegramId(db *sql.DB, tgid int) (*types.User, error) {
	user, dbId, err := GetUserByTelegramId(db, tgid)
	if err != nil {
		return nil, err
	}

	reminders, err := GetRemindersByUserId(db, dbId)
	if err != nil {
		return nil, err
	}

	user.Time = reminders
	return user, nil
}

func GetFullUserById(db *sql.DB, id int64) (*types.User, error) {
	user, err := GetUserById(db, id)
	if err != nil {
		return nil, err
	}

	reminders, err := GetRemindersByUserId(db, id)
	if err != nil {
		return nil, err
	}

	user.Time = reminders
	return user, nil
}

func DeleteReminderByTimestamp(db *sql.DB, time types.TimeEntry, id int64) error {
	query := `DELETE FROM reminders WHERE user_id = ? AND hour = ? AND minute = ?`
	result, err := db.Exec(query, id, time.Hour, time.Minute)
	if err != nil {
		return fmt.Errorf("ошибка удаления напоминания: %v", err)
	}

	rAff, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rAff == 0 {
		return fmt.Errorf("напоминание не найдено")
	}

	return nil
}

func DeleteReminderById(db *sql.DB, id int64) error {
query := `DELETE FROM reminders WHERE id = ?`
	result, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("ошибка удаления напоминания: %v", err)
	}

	rAff, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rAff == 0 {
		return fmt.Errorf("напоминание не найдено")
	}

	return nil
}

func DeleteAllReminders(db *sql.DB, id int64) error {
	query := `DELETE FROM reminders WHERE user_id = ?`
	_, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("ошибка удаления напоминания: %v", err)
	}

	return nil
}

func CountUserReminders(db *sql.DB, id int64) (int, error) {
	query := `SELECT COUNT(*) FROM reminders WHERE user_id = ?`
	
	var count int
	err := db.QueryRow(query, id).Scan(&count)
	if err != nil {
		return -1, err
	}

	return count, nil
}

func GetReminderById(db *sql.DB, id int) (int64, *types.TimeEntry, error) {
	query := `SELECT user_id, hour, minute FROM reminders WHERE id = ?`
	var time types.TimeEntry
	var tgId int64
	err := db.QueryRow(query, id).Scan(
		&tgId,
		&time.Hour,
		&time.Minute,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return -1, nil, nil
		}
		return -1, nil, err
	}

	return tgId, &time, nil
}

func UserExists(db *sql.DB, tgid int) bool {
	query := `SELECT COUNT(*) FROM users WHERE telegram_id = ?`

	var count int
	err := db.QueryRow(query, tgid).Scan(&count)
	if err != nil {
		log.Printf("Ошибка проверки существования пользователя: %v", err)
		return false
	}

	return count > 0
}

func GetRemindersWithID(db *sql.DB, userID int64) ([]types.ReminderWithID, error) {
	query := `SELECT id, hour, minute FROM reminders WHERE user_id = ? ORDER BY hour, minute`
    rows, err := db.Query(query, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var reminders []types.ReminderWithID
    for rows.Next() {
        var r types.ReminderWithID
        err := rows.Scan(&r.ID, &r.Reminder.Hour, &r.Reminder.Minute)
        if err != nil {
            return nil, err
        }
        reminders = append(reminders, r)
    }
    return reminders, rows.Err()
}