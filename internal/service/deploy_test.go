package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hellodeveye/postdare-go/internal/model"
)

func TestExecuteDeployKeepsSuccessAfterNotifyStage(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("noop", "true", true),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	svc.ExecuteTask(context.Background(), task.ID)

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskSuccess {
		t.Fatalf("expected success, got %s", got.Status)
	}
}

func TestExecuteDeployRunsDynamicStagesInOrder(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("checkout", "true", true),
			commandStage("disabled", "false", false),
			commandStageWithPolicy("flaky", "false", "", true),
			commandStage("ship", "true", true),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	svc.ExecuteTask(context.Background(), task.ID)

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskSuccess {
		t.Fatalf("expected success (flaky stage has continue_on_error), got %s", got.Status)
	}

	var stages []model.DeployTaskStage
	if err := svc.DB.Where("task_id = ?", task.ID).Order("id asc").Find(&stages).Error; err != nil {
		t.Fatal(err)
	}
	statusByName := map[string]string{}
	for _, s := range stages {
		statusByName[s.Name] = s.Status
	}
	if _, ok := statusByName["disabled"]; ok {
		t.Fatalf("disabled stage should not run, got stages %+v", statusByName)
	}
	if statusByName["checkout"] != model.StageSuccess {
		t.Fatalf("expected checkout success, got %q", statusByName["checkout"])
	}
	if statusByName["flaky"] != model.StageFailed {
		t.Fatalf("expected flaky failed, got %q", statusByName["flaky"])
	}
	if statusByName["ship"] != model.StageSuccess {
		t.Fatalf("expected ship success, got %q", statusByName["ship"])
	}
}

func TestExecuteDeployFailsWhenStageFailsWithoutContinueOnError(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("checkout", "true", true),
			commandStage("build", "false", true),
			commandStage("ship", "true", true),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	svc.ExecuteTask(context.Background(), task.ID)

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}

	var shipCount int64
	if err := svc.DB.Model(&model.DeployTaskStage{}).Where("task_id = ? AND name = ?", task.ID, "ship").Count(&shipCount).Error; err != nil {
		t.Fatal(err)
	}
	if shipCount != 0 {
		t.Fatalf("expected ship stage to be skipped after failure, got %d rows", shipCount)
	}
}

func TestFailedDeployRunsFailedOutboundWebhookStage(t *testing.T) {
	var webhookCalls int32
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("build", "false", true),
			outboundWebhookStage("outbound_webhook", webhookServer.URL, model.ProjectStageRunWhenFailed),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	svc.ExecuteTask(context.Background(), task.ID)

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if calls := atomic.LoadInt32(&webhookCalls); calls != 1 {
		t.Fatalf("expected outbound webhook to be called once, got %d", calls)
	}
}
