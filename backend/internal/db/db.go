package db

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/model"
)

const defaultAdminPassword = "admin123456"

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database.dsn is required")
	}
	database, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := database.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.DeployTask{},
		&model.DeployTaskStage{},
		&model.WebhookEvent{},
		&model.Setting{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err := seedDefaultAdmin(database); err != nil {
		return nil, err
	}
	return database, nil
}

func seedDefaultAdmin(database *gorm.DB) error {
	var count int64
	if err := database.Model(&model.User{}).Where("username = ?", "admin").Count(&count).Error; err != nil {
		return fmt.Errorf("count default admin: %w", err)
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}
	user := model.User{Username: "admin", PasswordHash: string(hash), Role: "admin"}
	if err := database.Create(&user).Error; err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}
	return nil
}
