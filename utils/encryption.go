package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

var encryptionKey []byte

// InitEncryption initializes the encryption key
func InitEncryption(keyStr string) error {
	var key []byte
	
	if keyStr == "" {
		// Generate a default key from a fixed seed (NOT recommended for production)
		seed := "fishing-platform-default-encryption-key-2024"
		hash := sha256.Sum256([]byte(seed))
		key = hash[:]
	} else {
		// Use provided key
		hash := sha256.Sum256([]byte(keyStr))
		key = hash[:]
	}
	
	encryptionKey = key
	return nil
}

// Encrypt encrypts plaintext using AES-GCM
func Encrypt(plaintext string) (string, error) {
	if encryptionKey == nil {
		return "", nil // No encryption initialized, return plaintext
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-GCM
func Decrypt(ciphertext string) (string, error) {
	if encryptionKey == nil {
		return ciphertext, nil // No encryption initialized, return as-is
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", nil // Not encrypted, return as-is
	}

	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", nil // Decryption failed, might not be encrypted
	}

	return string(plaintext), nil
}

// GenerateEncryptionKey generates a random encryption key for production
func GenerateEncryptionKey() string {
	key := make([]byte, 32) // 256 bits
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}