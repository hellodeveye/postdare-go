package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/model"
	"postdare-go/backend/internal/service"
	"postdare-go/backend/internal/sse"
)

func TestUpdateProjectPersistsDeployStages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.PATCH("/projects/:project_id", h.UpdateProject)

	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      "/data/app",
	}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"deploy_stages":[{"name":"build","type":"command","enabled":true,"config":{"command":"make"}},{"name":"ship","type":"command","enabled":true,"continue_on_error":true,"config":{"command":"./deploy.sh"}}]}`
	req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}

	var reloaded model.Project
	if err := database.First(&reloaded, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Stages) != 2 {
		t.Fatalf("expected 2 stages persisted, got %d (%+v)", len(reloaded.Stages), reloaded.Stages)
	}
	var buildConfig model.CommandStageConfig
	if err := json.Unmarshal(reloaded.Stages[0].Config, &buildConfig); err != nil {
		t.Fatal(err)
	}
	if reloaded.Stages[0].Name != "build" || reloaded.Stages[0].Type != model.ProjectStageTypeCommand || buildConfig.Command != "make" || !reloaded.Stages[0].Enabled {
		t.Fatalf("unexpected first stage: %+v", reloaded.Stages[0])
	}
	if reloaded.Stages[1].Name != "ship" || !reloaded.Stages[1].ContinueOnError {
		t.Fatalf("unexpected second stage: %+v", reloaded.Stages[1])
	}

	// Emptying the pipeline must persist as an empty list, not be ignored.
	req = httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(`{"deploy_stages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 on clear, got %d: %s", res.Code, res.Body.String())
	}
	var cleared model.Project
	if err := database.First(&cleared, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(cleared.Stages) != 0 {
		t.Fatalf("expected stages cleared, got %+v", cleared.Stages)
	}
}

func TestUpdateProjectPreservesMaskedOutboundWebhookURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.PATCH("/projects/:project_id", h.UpdateProject)

	realURL := "https://hooks.example.com/notify"
	project := model.Project{
		Name:        "app",
		ProjectKey:  "app",
		GitProvider: model.GitProviderGitHub,
		Branch:      "main",
		AppDir:      "/data/app",
		Stages: []model.ProjectStage{{
			Name:    "outbound_webhook",
			Type:    model.ProjectStageTypeOutboundWebhook,
			Enabled: true,
			RunWhen: model.ProjectStageRunWhenAlways,
			Config:  testStageConfig(model.OutboundWebhookStageConfig{URL: realURL, Template: "dingtalk_text"}),
		}},
	}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"deploy_stages":[{"name":"outbound_webhook","type":"outbound_webhook","enabled":true,"run_when":"always","continue_on_error":true,"config":{"url":"htt******ify","template":"dingtalk_text","message_template":"hello"}}]}`
	req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), realURL) {
		t.Fatalf("expected response to mask outbound webhook URL: %s", res.Body.String())
	}

	var reloaded model.Project
	if err := database.First(&reloaded, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	var gotConfig model.OutboundWebhookStageConfig
	if err := json.Unmarshal(reloaded.Stages[0].Config, &gotConfig); err != nil {
		t.Fatal(err)
	}
	if gotConfig.URL != realURL {
		t.Fatalf("expected stored URL to be preserved, got %q", gotConfig.URL)
	}
	if gotConfig.MessageTemplate != "hello" {
		t.Fatalf("expected other config fields to update, got %+v", gotConfig)
	}
}

func TestUpdateProjectRejectsStageWithoutName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.PATCH("/projects/:project_id", h.UpdateProject)

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main",AppDir: "/data/app"}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(`{"deploy_stages":[{"name":"","type":"command","enabled":true,"config":{"command":"make"}}]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for nameless stage, got %d: %s", res.Code, res.Body.String())
	}

	var reloaded model.Project
	if err := database.First(&reloaded, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Stages) != 0 {
		t.Fatalf("invalid update should not persist stages, got %+v", reloaded.Stages)
	}
}

func TestUpdateProjectRejectsEnabledOutboundWebhookWithoutURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.PATCH("/projects/:project_id", h.UpdateProject)

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main",AppDir: "/data/app"}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"deploy_stages":[{"name":"outbound_webhook","type":"outbound_webhook","enabled":true,"run_when":"always","config":{"template":"dingtalk_text"}}]}`
	req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for outbound webhook without URL, got %d: %s", res.Code, res.Body.String())
	}
}

func TestUpdateProjectRejectsEnabledHealthCheckWithoutURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.PATCH("/projects/:project_id", h.UpdateProject)

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main",AppDir: "/data/app"}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"deploy_stages":[{"name":"health_check","type":"health_check","enabled":true,"config":{}}]}`
	req := httptest.NewRequest(http.MethodPatch, "/projects/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for health check without URL, got %d: %s", res.Code, res.Body.String())
	}
}

func testStageConfig(value interface{}) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestCancelDeployTaskErrorResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.DeployTask{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.POST("/deploy-tasks/:task_id/cancel", h.CancelDeployTask)

	req := httptest.NewRequest(http.MethodPost, "/deploy-tasks/404/cancel", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", res.Code, res.Body.String())
	}

	task := model.DeployTask{ProjectID: 1, TriggerType: model.TriggerManual, Status: model.TaskSuccess}
	if err := database.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/deploy-tasks/1/cancel", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", res.Code, res.Body.String())
	}
}

func TestSanitizeLogTextStripsANSIEscapeSequences(t *testing.T) {
	raw := "\x1b[2m22:20:54.747\x1b[0m \x1b[32mINFO\x1b[0m \x1b[1mxianhu-chaos listening\x1b[0m addr=\x1b[36m:18080\x1b[0m\n中文日志"

	got := sanitizeLogText(raw)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("expected ANSI escapes to be stripped, got %q", got)
	}
	if !strings.Contains(got, "INFO xianhu-chaos listening addr=:18080") {
		t.Fatalf("expected readable log content to remain, got %q", got)
	}
	if !strings.Contains(got, "中文日志") {
		t.Fatalf("expected non-ASCII log content to remain, got %q", got)
	}
}
