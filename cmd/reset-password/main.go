package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if _, err := config.BootstrapRuntimeConfig(); err != nil {
		log.Fatalf("Failed to load config.yaml: %v", err)
	}

	dbPath := config.DatabasePath()
	if err := config.InitializeDatabase(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := models.AutoMigrate(config.GetDB()); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	db := config.GetDB()

	password, err := generateStrongPassword(20)
	if err != nil {
		log.Fatalf("Failed to generate strong password: %v", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		admin = models.User{
			Username: "admin",
			Password: string(hashedPassword),
		}
		if err := db.Create(&admin).Error; err != nil {
			log.Fatalf("Failed to create admin user: %v", err)
		}
	} else {
		admin.Password = string(hashedPassword)
		if err := db.Save(&admin).Error; err != nil {
			log.Fatalf("Failed to update admin password: %v", err)
		}
	}

	passwordFile, err := writePasswordFile(dbPath, password)
	if err != nil {
		log.Fatalf("Failed to save password file: %v", err)
	}

	fmt.Println("Admin password reset successfully!")
	fmt.Println("Username: admin")
	fmt.Printf("Password: %s\n", password)
	fmt.Printf("Password saved to: %s\n", passwordFile)
}

func generateStrongPassword(length int) (string, error) {
	if length < 12 {
		length = 12
	}

	lower := "abcdefghijklmnopqrstuvwxyz"
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits := "0123456789"
	symbols := "!@#$%^&*()-_=+[]{}"
	allChars := lower + upper + digits + symbols

	passwordRunes := make([]byte, 0, length)
	requiredSets := []string{lower, upper, digits, symbols}
	for _, charset := range requiredSets {
		ch, err := randomChar(charset)
		if err != nil {
			return "", err
		}
		passwordRunes = append(passwordRunes, ch)
	}

	for len(passwordRunes) < length {
		ch, err := randomChar(allChars)
		if err != nil {
			return "", err
		}
		passwordRunes = append(passwordRunes, ch)
	}

	if err := shuffleBytes(passwordRunes); err != nil {
		return "", err
	}

	return string(passwordRunes), nil
}

func randomChar(charset string) (byte, error) {
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	if err != nil {
		return 0, err
	}
	return charset[idx.Int64()], nil
}

func shuffleBytes(items []byte) error {
	for i := len(items) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		items[i], items[j.Int64()] = items[j.Int64()], items[i]
	}
	return nil
}

func writePasswordFile(dbPath string, password string) (string, error) {
	dataDir := filepath.Dir(dbPath)
	if dataDir == "." || dataDir == "" {
		dataDir = "./data"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}

	passwordFile := filepath.Join(dataDir, "admin_password.txt")
	content := fmt.Sprintf("Fishing Platform Admin Credentials\n================================\nUsername: admin\nPassword: %s\n\nIMPORTANT: Please save this password and delete this file after logging in.\n", password)
	if err := os.WriteFile(passwordFile, []byte(content), 0600); err != nil {
		return "", err
	}

	return passwordFile, nil
}
