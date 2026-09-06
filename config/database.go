package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitializeDatabase initializes database connection
func InitializeDatabase(dbPath string) error {
	var err error

	// Create directory for database if it doesn't exist
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Configure GORM - use silent mode to avoid debug output
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	// Open database connection
	DB, err = gorm.Open(sqlite.Open(dbPath), config)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Return DB, migrations will be run by caller
	return nil
}

// RunMigrations runs database migrations (called from models package to avoid circular dependency)
func RunMigrations() error {
	// This function is called from models package
	// to avoid circular dependency
	return nil
}

// GetDB returns database instance
func GetDB() *gorm.DB {
	return DB
}
