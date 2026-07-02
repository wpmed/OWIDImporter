package models

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/wpmed-videowiki/OWIDImporter/env"
)

var db *sql.DB

func Init() {
	if db == nil {
		dbDir := env.GetEnv().OWID_DB_DIR
		fmt.Println("DB DIR", dbDir)
		sqlitSettings := fmt.Sprintf("file:%s/db.db?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on", dbDir)
		db1, err := sql.Open("sqlite3", sqlitSettings)
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
