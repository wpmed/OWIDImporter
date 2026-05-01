package models

import (
	"database/sql"
	"log"
)

var db *sql.DB

func Init() {
	if db == nil {
		db1, err := sql.Open("sqlite3", "file:db.db?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on")
		if err != nil {
			log.Fatal(err)
		}
		db1.SetMaxOpenConns(1)
		db1.SetMaxIdleConns(1)
		if err := db1.Ping(); err != nil {
			log.Fatal(err)
		}
		db = db1
	}

	initUserTable()
	initTaskTable()
	initTaskProcessTable()
}
