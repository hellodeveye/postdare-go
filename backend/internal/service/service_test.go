package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/model"
	"postdare-go/backend/internal/sse"
)

func TestExecuteDeployKeepsSuccessAfterNotifyStage(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
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

func TestExecuteRollbackKeepsRollbackedAfterNotifyStage(t *testing.T) {
	var webhookCalls int32
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer webhookServer.Close()

	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
		AppDir:      t.TempDir(),
		RollbackCmd: "true",
		Stages: []model.ProjectStage{
			outboundWebhookStage("outbound_webhook", webhookServer.URL, model.ProjectStageRunWhenAlways),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerRollback, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	svc.ExecuteTask(context.Background(), task.ID)

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskRollbacked {
		t.Fatalf("expected rollbacked, got %s", got.Status)
	}
	if calls := atomic.LoadInt32(&webhookCalls); calls != 1 {
		t.Fatalf("expected rollback outbound webhook to be called once, got %d", calls)
	}
}

func TestExecuteDeployRunsDynamicStagesInOrder(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
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
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
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
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
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

func TestCancelTaskReturnsStateErrors(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CancelTask(context.Background(), 999); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	task := model.DeployTask{ProjectID: 1, TriggerType: model.TriggerManual, Status: model.TaskSuccess}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelTask(context.Background(), task.ID); err != ErrTaskNotCancelable {
		t.Fatalf("expected ErrTaskNotCancelable, got %v", err)
	}
}

func TestShutdownRejectsNewDeployTasks(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
		AppDir:      t.TempDir(),
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDeployTask(context.Background(), project, model.TriggerManual, nil); err != ErrServiceShuttingDown {
		t.Fatalf("expected ErrServiceShuttingDown, got %v", err)
	}
}

func TestStartTaskRegistersCancelBeforeLaunch(t *testing.T) {
	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("wait", "sleep 2", true),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.startTask(task.ID); err != nil {
		t.Fatal(err)
	}
	svc.cancelMu.Lock()
	_, registered := svc.cancels[task.ID]
	svc.cancelMu.Unlock()
	if !registered {
		t.Fatal("task cancel was not registered before launch returned")
	}
	_ = svc.Shutdown(context.Background())
}

func TestCanceledDeployDoesNotSendFailureNotification(t *testing.T) {
	var notifyCalls int32
	notifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&notifyCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer notifyServer.Close()

	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("wait", "sleep 30", true),
			outboundWebhookStage("outbound_webhook", notifyServer.URL, model.ProjectStageRunWhenAlways),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.startTask(task.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var current model.DeployTask
		if err := svc.DB.First(&current, task.ID).Error; err == nil && current.Status == model.TaskRunning {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := svc.CancelTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&notifyCalls); got != 0 {
		t.Fatalf("expected no failure notification for canceled task, got %d", got)
	}
}

func TestCanceledHealthCheckDoesNotSendFailureNotification(t *testing.T) {
	var notifyCalls int32
	notifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&notifyCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer notifyServer.Close()

	healthRequestStarted := make(chan struct{})
	var closeStarted sync.Once
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeStarted.Do(func() { close(healthRequestStarted) })
		<-r.Context().Done()
	}))
	defer healthServer.Close()

	svc := newTestService(t)
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		RepoURL:     "git@example.com:app.git",
		Branch:      "main",
		RepoDir:     t.TempDir(),
		AppDir:      t.TempDir(),
		Stages: []model.ProjectStage{
			commandStage("noop", "true", true),
			healthCheckStage("health_check", healthServer.URL, ""),
			outboundWebhookStage("outbound_webhook", notifyServer.URL, model.ProjectStageRunWhenAlways),
		},
	}
	if err := svc.DB.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, GitProvider: project.GitProvider, Branch: project.Branch, Status: model.TaskPending}
	if err := svc.DB.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.startTask(task.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-healthRequestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("health check did not start")
	}
	if err := svc.CancelTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&notifyCalls); got != 0 {
		t.Fatalf("expected no failure notification for canceled health check, got %d", got)
	}

	var got model.DeployTask
	if err := svc.DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskCanceled {
		t.Fatalf("expected canceled task, got %s", got.Status)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "postdare-go-test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}, &model.DeployTask{}, &model.DeployTaskStage{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Deploy.LogDir = t.TempDir()
	cfg.Deploy.CommandTimeoutMinutes = 1
	cfg.AppLog.MaxTailLines = 500
	cfg.AppLog.MaxAllowedLines = 5000
	svc := New(database, cfg, sse.NewHub(), zap.NewNop())
	return svc
}

func commandStage(name string, command string, enabled bool) model.ProjectStage {
	return model.ProjectStage{
		Name:    name,
		Type:    model.ProjectStageTypeCommand,
		Enabled: enabled,
		Config:  rawStageConfig(model.CommandStageConfig{Command: command}),
	}
}

func commandStageWithPolicy(name string, command string, runWhen string, continueOnError bool) model.ProjectStage {
	stage := commandStage(name, command, true)
	stage.RunWhen = runWhen
	stage.ContinueOnError = continueOnError
	return stage
}

func healthCheckStage(name string, url string, runWhen string) model.ProjectStage {
	return model.ProjectStage{
		Name:    name,
		Type:    model.ProjectStageTypeHealthCheck,
		Enabled: true,
		RunWhen: runWhen,
		Config:  rawStageConfig(model.HealthCheckStageConfig{URL: url}),
	}
}

func outboundWebhookStage(name string, url string, runWhen string) model.ProjectStage {
	return model.ProjectStage{
		Name:            name,
		Type:            model.ProjectStageTypeOutboundWebhook,
		Enabled:         true,
		RunWhen:         runWhen,
		ContinueOnError: true,
		Config: rawStageConfig(model.OutboundWebhookStageConfig{
			URL:      url,
			Template: "dingtalk_text",
		}),
	}
}

func rawStageConfig(value interface{}) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
