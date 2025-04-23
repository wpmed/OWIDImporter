package models

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type User struct {
	ID                  string `json:"id"`
	Username            string `json:"username"`
	ResourceOwnerSecret string `json:"resource_owner_secret"`
	ResourceOwnerKey    string `json:"resource_owner_key"`
}

func NewUser(username, resourceOwnerKey, resourceOwnerSecret string) (*User, error) {
	user := User{
		ID:                  uuid.New().String(),
		Username:            username,
		ResourceOwnerSecret: resourceOwnerSecret,
		ResourceOwnerKey:    resourceOwnerKey,
	}
	stmt, err := db.Prepare("INSERT INTO user (id, username,resource_owner_key, resource_owner_secret) VALUES (?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result, err := stmt.Exec(user.ID, user.Username, user.ResourceOwnerKey, user.ResourceOwnerSecret)
	if err != nil {
		return nil, err
	}
	fmt.Println("CREATE USER ", result)

	return &user, nil
}

func (user *User) Update() error {
	stmt, err := db.Prepare("UPDATE user SET resource_owner_key=?, resource_owner_secret=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(user.ResourceOwnerKey, user.ResourceOwnerSecret, user.ID)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK ", result)

	return nil
}

func FindUserByUsername(username string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, username, resource_owner_key, resource_owner_secret FROM user where username=?", username).Scan(&user.ID, &user.Username, &user.ResourceOwnerKey, &user.ResourceOwnerSecret)
	fmt.Println("Query result: ", user, err)
	if err != nil {
		println("Error scaning for username", username, err)
		return nil, fmt.Errorf("Cannot find requested record")
	}

	return &user, nil
}

func initUserTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS user (
		id VARCHAR(255) PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		resource_owner_key TEXT,
		resource_owner_secret TEXT
	);`)
	if err != nil {
		log.Fatal(err)
	}
}
