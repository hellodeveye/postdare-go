package handler

import (
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
