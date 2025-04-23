package models

import (
	"database/sql"
	"log"
)

var db *sql.DB

func Init() {
	if db == nil {
		db1, err := sql.Open("sqlite3", "./db.db")
		if err != nil {
			log.Fatal(err)
		}
		db = db1
	}

	initUserTable()
	initTaskTable()
	initTaskProcessTable()
}
