package db

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/model"
	"postdare-go/backend/internal/service"
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
		&model.DeployArtifact{},
		&model.WebhookEvent{},
		&model.Setting{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err := seedDefaultAdmin(database); err != nil {
		return nil, err
	}
	if err := backfillDeployStages(database); err != nil {
		return nil, err
	}
	return database, nil
}

// backfillDeployStages populates the new deploy_stages pipeline for projects created
// before dynamic stages existed. For each project without a pipeline, it derives one
// from the legacy per-command fields so their deploy behavior is unchanged.
func backfillDeployStages(database *gorm.DB) error {
	var projects []model.Project
	if err := database.Find(&projects).Error; err != nil {
		return fmt.Errorf("load projects for stage backfill: %w", err)
	}
	for i := range projects {
		if len(projects[i].Stages) > 0 {
			continue
		}
		stages := service.LegacyDeployStages(projects[i])
		if len(stages) == 0 {
			continue
		}
		if err := database.Model(&projects[i]).Update("stages", stages).Error; err != nil {
			return fmt.Errorf("backfill deploy stages for project %d: %w", projects[i].ID, err)
		}
	}
	return nil
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
