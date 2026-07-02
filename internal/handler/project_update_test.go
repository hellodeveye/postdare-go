package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hellodeveye/postdare-go/internal/config"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/service"
	"github.com/hellodeveye/postdare-go/internal/sse"
	"go.uber.org/zap"
	"gorm.io/gorm"
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

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main", AppDir: "/data/app"}
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

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main", AppDir: "/data/app"}
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

	project := model.Project{Name: "app", ProjectKey: "app", GitProvider: model.GitProviderGitHub, Branch: "main", AppDir: "/data/app"}
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
