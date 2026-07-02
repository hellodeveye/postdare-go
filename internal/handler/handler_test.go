package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/hellodeveye/postdare-go/internal/config"
	"github.com/hellodeveye/postdare-go/internal/middleware"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/service"
	"github.com/hellodeveye/postdare-go/internal/sse"
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

func TestDeleteProjectReturnsNotFound(t *testing.T) {
	_, router := setupDeleteProjectTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/projects/404", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", res.Code, res.Body.String())
	}
}

func TestDeleteProjectBlocksActiveTask(t *testing.T) {
	database, router := setupDeleteProjectTest(t)
	project := createHandlerTestProject(t, database, "app")
	task := model.DeployTask{ProjectID: project.ID, TriggerType: model.TriggerManual, Status: model.TaskRunning}
	if err := database.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	stage := model.DeployTaskStage{TaskID: task.ID, Name: "deploy", Status: model.StageRunning}
	if err := database.Create(&stage).Error; err != nil {
		t.Fatal(err)
	}
	eventProjectID := project.ID
	event := model.WebhookEvent{Provider: model.GitProviderGitHub, ProjectID: &eventProjectID, ProjectKey: project.ProjectKey}
	if err := database.Create(&event).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/projects/1", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", res.Code, res.Body.String())
	}

	assertRowCount(t, database, &model.Project{}, "id = ?", 1, project.ID)
	assertRowCount(t, database, &model.DeployTask{}, "id = ?", 1, task.ID)
	assertRowCount(t, database, &model.DeployTaskStage{}, "id = ?", 1, stage.ID)
	assertRowCount(t, database, &model.WebhookEvent{}, "id = ?", 1, event.ID)
}

func TestDeleteProjectCascadesRelatedRecordsAndKeepsLogFile(t *testing.T) {
	database, router := setupDeleteProjectTest(t)
	project := createHandlerTestProject(t, database, "app")
	otherProject := createHandlerTestProject(t, database, "other")
	logFile := filepath.Join(t.TempDir(), "deploy.log")
	if err := os.WriteFile(logFile, []byte("deploy output\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tasks := []model.DeployTask{
		{ProjectID: project.ID, TriggerType: model.TriggerManual, Status: model.TaskSuccess, LogFile: logFile},
		{ProjectID: project.ID, TriggerType: model.TriggerRollback, Status: model.TaskFailed},
		{ProjectID: otherProject.ID, TriggerType: model.TriggerManual, Status: model.TaskSuccess},
	}
	if err := database.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	taskA, taskB, otherTask := tasks[0], tasks[1], tasks[2]

	stages := []model.DeployTaskStage{
		{TaskID: taskA.ID, Name: "build", Status: model.StageSuccess},
		{TaskID: taskB.ID, Name: "rollback", Status: model.StageFailed},
		{TaskID: otherTask.ID, Name: "build", Status: model.StageSuccess},
	}
	if err := database.Create(&stages).Error; err != nil {
		t.Fatal(err)
	}
	projectID := project.ID
	otherProjectID := otherProject.ID
	events := []model.WebhookEvent{
		{Provider: model.GitProviderGitHub, ProjectID: &projectID, ProjectKey: project.ProjectKey},
		{Provider: model.GitProviderGitee, ProjectKey: project.ProjectKey},
		{Provider: model.GitProviderGitHub, ProjectID: &otherProjectID, ProjectKey: otherProject.ProjectKey},
	}
	if err := database.Create(&events).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/projects/1", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", res.Code, res.Body.String())
	}

	assertRowCount(t, database, &model.Project{}, "id = ?", 0, project.ID)
	assertRowCount(t, database, &model.Project{}, "id = ?", 1, otherProject.ID)
	assertRowCount(t, database, &model.DeployTask{}, "project_id = ?", 0, project.ID)
	assertRowCount(t, database, &model.DeployTask{}, "project_id = ?", 1, otherProject.ID)
	assertRowCount(t, database, &model.DeployTaskStage{}, "task_id IN ?", 0, []uint64{taskA.ID, taskB.ID})
	assertRowCount(t, database, &model.DeployTaskStage{}, "task_id = ?", 1, otherTask.ID)
	assertRowCount(t, database, &model.WebhookEvent{}, "project_id = ? OR project_key = ?", 0, project.ID, project.ProjectKey)
	assertRowCount(t, database, &model.WebhookEvent{}, "project_id = ? OR project_key = ?", 1, otherProject.ID, otherProject.ProjectKey)
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("expected physical deploy log file to remain, got %v", err)
	}
}

func setupDeleteProjectTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Project{}, &model.DeployTask{}, &model.DeployTaskStage{}); err != nil {
		t.Fatal(err)
	}
	// SQLite requires globally unique index names, so this test creates the
	// webhook table directly instead of AutoMigrating two idx_created_at models.
	if err := database.Exec(`CREATE TABLE webhook_events (
		id integer PRIMARY KEY AUTOINCREMENT,
		provider text NOT NULL,
		project_id integer,
		project_key text,
		event_type text,
		branch text,
		commit_id text,
		commit_message text,
		commit_author text,
		delivery_id text,
		signature_valid numeric NOT NULL DEFAULT false,
		handled numeric NOT NULL DEFAULT false,
		ignored_reason text,
		raw_payload json,
		created_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := service.New(database, cfg, sse.NewHub(), zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: sse.NewHub()}
	router := gin.New()
	router.DELETE("/projects/:project_id", h.DeleteProject)
	return database, router
}

func createHandlerTestProject(t *testing.T, database *gorm.DB, key string) model.Project {
	t.Helper()
	project := model.Project{Name: key, ProjectKey: key, GitProvider: model.GitProviderGitHub, Branch: "main", AppDir: "/data/" + key}
	if err := database.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	return project
}

func assertRowCount(t *testing.T, database *gorm.DB, modelValue interface{}, query string, want int64, args ...interface{}) {
	t.Helper()
	var got int64
	if err := database.Model(modelValue).Where(query, args...).Count(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected %s count %d, got %d", query, want, got)
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

func TestPasswordChangeRequiredFlow(t *testing.T) {
	database, router := setupAuthFlowTest(t, true)
	token := loginForToken(t, router, "admin", "old-password")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected auth/me to be allowed, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"must_change_password":true`) {
		t.Fatalf("expected auth/me to include must_change_password=true: %s", res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("expected password change required, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", strings.NewReader(`{"old_password":"bad-password","new_password":"new-password"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong old password, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", strings.NewReader(`{"old_password":"old-password","new_password":"short"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short new password, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected password update success, got %d: %s", res.Code, res.Body.String())
	}

	var admin model.User
	if err := database.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if admin.MustChangePassword {
		t.Fatal("expected must_change_password to be cleared")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("new-password")); err != nil {
		t.Fatal("new password does not match stored hash")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected projects to be allowed after password update, got %d: %s", res.Code, res.Body.String())
	}
}

func setupAuthFlowTest(t *testing.T, mustChangePassword bool) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.DeployTask{},
		&model.DeployTaskStage{},
		&model.WebhookEvent{},
		&model.Setting{},
	); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.User{
		Username:           "admin",
		PasswordHash:       string(hash),
		Role:               "admin",
		MustChangePassword: mustChangePassword,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHours: 72}}
	hub := sse.NewHub()
	svc := service.New(database, cfg, hub, zap.NewNop())
	h := &Handler{DB: database, Config: cfg, Service: svc, Hub: hub}
	router := gin.New()
	router.Use(middleware.RequestID())
	RegisterRoutes(router, h)
	return database, router
}

func loginForToken(t *testing.T, router *gin.Engine, username string, password string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected login success, got %d: %s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Token == "" {
		t.Fatal("login response did not include token")
	}
	return body.Data.Token
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
