package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"postdare-go/backend/internal/model"
)

func TestSendOutboundWebhookRendersFeishuTextTemplate(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	project := model.Project{Name: "app"}
	task := model.DeployTask{ID: 42, Status: model.TaskFailed, CurrentStage: "build", FailReason: "exit status 1"}
	err := New(zap.NewNop()).SendOutboundWebhook(project, task, model.OutboundWebhookStageConfig{
		URL:             server.URL,
		Template:        TemplateFeishuText,
		MessageTemplate: "项目={{ .Project.Name }} 状态={{ .Task.Status }} 阶段={{ .Task.CurrentStage }}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["msg_type"] != "text" {
		t.Fatalf("expected feishu text payload, got %+v", got)
	}
	content, ok := got["content"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content object, got %+v", got["content"])
	}
	text, _ := content["text"].(string)
	if !strings.Contains(text, "项目=app") || !strings.Contains(text, "状态=failed") || !strings.Contains(text, "阶段=build") {
		t.Fatalf("unexpected rendered text: %q", text)
	}
}
