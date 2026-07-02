package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"postdare-go/backend/internal/model"
)

func main() {
	from := flag.String("from", "", "source MySQL DSN")
	to := flag.String("to", "", "target SQLite database path")
	flag.Parse()
	if *from == "" || *to == "" {
		flag.Usage()
		os.Exit(2)
	}

	source, err := gorm.Open(mysql.Open(*from), &gorm.Config{})
	if err != nil {
		log.Fatalf("open source mysql: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*to), 0o755); err != nil {
		log.Fatalf("create target directory: %v", err)
	}
	target, err := gorm.Open(sqlite.Open(sqliteDSN(*to)), &gorm.Config{})
	if err != nil {
		log.Fatalf("open target sqlite: %v", err)
	}
	if err := autoMigrate(target); err != nil {
		log.Fatalf("migrate target sqlite: %v", err)
	}
	if err := ensureTargetEmpty(target); err != nil {
		log.Fatal(err)
	}

	counts := map[string]int{}
	err = target.Transaction(func(tx *gorm.DB) error {
		var err error
		counts["users"], err = copyTable[model.User](source, tx)
		if err != nil {
			return fmt.Errorf("copy users: %w", err)
		}
		counts["projects"], err = copyTable[model.Project](source, tx)
		if err != nil {
			return fmt.Errorf("copy projects: %w", err)
		}
		counts["deploy_tasks"], err = copyTable[model.DeployTask](source, tx)
		if err != nil {
			return fmt.Errorf("copy deploy_tasks: %w", err)
		}
		counts["deploy_task_stages"], err = copyTable[model.DeployTaskStage](source, tx)
		if err != nil {
			return fmt.Errorf("copy deploy_task_stages: %w", err)
		}
		counts["webhook_events"], err = copyTable[model.WebhookEvent](source, tx)
		if err != nil {
			return fmt.Errorf("copy webhook_events: %w", err)
		}
		counts["settings"], err = copyTable[model.Setting](source, tx)
		if err != nil {
			return fmt.Errorf("copy settings: %w", err)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("copy complete")
	for _, table := range []string{"users", "projects", "deploy_tasks", "deploy_task_stages", "webhook_events", "settings"} {
		fmt.Printf("%s: %d\n", table, counts[table])
	}
}

func autoMigrate(database *gorm.DB) error {
	return database.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.DeployTask{},
		&model.DeployTaskStage{},
		&model.WebhookEvent{},
		&model.Setting{},
	)
}

func ensureTargetEmpty(database *gorm.DB) error {
	checks := []struct {
		name  string
		model interface{}
	}{
		{"users", &model.User{}},
		{"projects", &model.Project{}},
		{"deploy_tasks", &model.DeployTask{}},
		{"deploy_task_stages", &model.DeployTaskStage{}},
		{"webhook_events", &model.WebhookEvent{}},
		{"settings", &model.Setting{}},
	}
	for _, check := range checks {
		var count int64
		if err := database.Model(check.model).Count(&count).Error; err != nil {
			return fmt.Errorf("count target %s: %w", check.name, err)
		}
		if count > 0 {
			return fmt.Errorf("target sqlite is not empty: %s has %d rows", check.name, count)
		}
	}
	return nil
}

func copyTable[T any](source *gorm.DB, target *gorm.DB) (int, error) {
	var rows []T
	if err := source.Order("id asc").Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := target.CreateInBatches(rows, 200).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}
