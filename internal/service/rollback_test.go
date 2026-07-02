package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hellodeveye/postdare-go/internal/model"
)

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
		Branch:      "main",
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
