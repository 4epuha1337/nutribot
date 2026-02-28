package database

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"nutribot/types"
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

func CreateUser(db *sql.DB, tgid int, chatid int, name string, age string, offset int, state int) (int64, error) {
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