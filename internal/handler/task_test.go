package handler

import (
	"net/http"
	"net/http/httptest"
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
