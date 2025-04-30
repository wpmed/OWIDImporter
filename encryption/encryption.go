package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"

	"github.com/wpmed-videowiki/OWIDImporter/env"
)

// Encrypt encrypts plaintext string using AES-GCM
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	envKey := env.GetEnv().OWID_ENCRYPTION_KEY
	encryptionKey, err := hex.DecodeString(envKey)
	if err != nil {
		log.Fatalf("Invalid encryption key format. Must be hex-encoded: %v", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext string using AES-GCM
func Decrypt(encryptedString string) (string, error) {
	if encryptedString == "" {
		return "", nil
	}

	envKey := env.GetEnv().OWID_ENCRYPTION_KEY
	encryptionKey, err := hex.DecodeString(envKey)
	if err != nil {
		log.Fatalf("Invalid encryption key format. Must be hex-encoded: %v", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedString)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
