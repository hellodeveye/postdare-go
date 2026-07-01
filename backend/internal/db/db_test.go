package db

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"postdare-go/backend/internal/model"
)

func TestBackfillDeployStagesSerializesLegacyStages(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}

	project := model.Project{
		Name:        "legacy",
		ProjectKey:  "legacy",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "https://example.com/repo.git",
		Branch:      "main",
		RepoDir:     "/tmp/repo",
		AppDir:      "/tmp/app",
		PullCmd:     "git pull",
		BuildCmd:    "make build",
		DeployCmd:   "./deploy.sh",
	}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	if err := backfillDeployStages(database); err != nil {
		t.Fatalf("backfillDeployStages returned error: %v", err)
	}

	var reloaded model.Project
	if err := database.First(&reloaded, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Stages) != 3 {
		t.Fatalf("expected 3 backfilled stages, got %d (%+v)", len(reloaded.Stages), reloaded.Stages)
	}
	if reloaded.Stages[0].Name != "pull_code" || reloaded.Stages[0].Command != "git pull" {
		t.Fatalf("unexpected first stage: %+v", reloaded.Stages[0])
	}
}
