package models

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/wpmed-videowiki/OWIDImporter/encryption"
)

type User struct {
	ID                  string `json:"id"`
	Username            string `json:"username"`
	ResourceOwnerSecret string `json:"resource_owner_secret"` // This will be encrypted in the database
	ResourceOwnerKey    string `json:"resource_owner_key"`    // This will be encrypted in the database
}

func NewUser(username, resourceOwnerKey, resourceOwnerSecret string) (*User, error) {
	// Create user object
	user := User{
		ID:                  uuid.New().String(),
		Username:            username,
		ResourceOwnerSecret: resourceOwnerSecret,
		ResourceOwnerKey:    resourceOwnerKey,
	}

	// Encrypt sensitive data
	encryptedKey, err := encryption.Encrypt(resourceOwnerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt resource owner key: %w", err)
	}

	encryptedSecret, err := encryption.Encrypt(resourceOwnerSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt resource owner secret: %w", err)
	}

	// Insert into database with encrypted values
	stmt, err := db.Prepare("INSERT INTO user (id, username, resource_owner_key, resource_owner_secret) VALUES (?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result, err := stmt.Exec(user.ID, user.Username, encryptedKey, encryptedSecret)
	if err != nil {
		return nil, err
	}

	fmt.Println("CREATE USER ", result)
	return &user, nil
}

func (user *User) Update() error {
	// Encrypt sensitive data before updating
	encryptedKey, err := encryption.Encrypt(user.ResourceOwnerKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt resource owner key: %w", err)
	}

	encryptedSecret, err := encryption.Encrypt(user.ResourceOwnerSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt resource owner secret: %w", err)
	}

	// Update database with encrypted values
	stmt, err := db.Prepare("UPDATE user SET resource_owner_key=?, resource_owner_secret=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(encryptedKey, encryptedSecret, user.ID)
	if err != nil {
		return err
	}

	fmt.Println("UPDATED USER ", result)
	return nil
}

func FindUserByUsername(username string) (*User, error) {
	var user User
	var encryptedKey, encryptedSecret string

	err := db.QueryRow("SELECT id, username, resource_owner_key, resource_owner_secret FROM user WHERE username=?", username).
		Scan(&user.ID, &user.Username, &encryptedKey, &encryptedSecret)
	if err != nil {
		println("Error scanning for username", username, err)
		return nil, fmt.Errorf("cannot find requested record")
	}

	// Decrypt data
	if encryptedKey != "" {
		decryptedKey, err := encryption.Decrypt(encryptedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt resource owner key: %w", err)
		}
		user.ResourceOwnerKey = decryptedKey
	}

	if encryptedSecret != "" {
		decryptedSecret, err := encryption.Decrypt(encryptedSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt resource owner secret: %w", err)
		}
		user.ResourceOwnerSecret = decryptedSecret
	}

	fmt.Println("Query result: ", user.ID, user.Username, "Owner Key: [REDACTED]", "Secret: [REDACTED]")
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
