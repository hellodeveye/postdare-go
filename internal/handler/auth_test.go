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
	"github.com/hellodeveye/postdare-go/internal/middleware"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/service"
	"github.com/hellodeveye/postdare-go/internal/sse"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

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
