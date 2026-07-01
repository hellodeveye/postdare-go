package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/middleware"
	"postdare-go/backend/internal/model"
	"postdare-go/backend/internal/service"
	"postdare-go/backend/internal/sse"
	"postdare-go/backend/internal/util"
	"postdare-go/backend/internal/webhook"
)

type Handler struct {
	DB      *gorm.DB
	Config  *config.Config
	Service *service.Service
	Hub     *sse.Hub
}

var ansiEscapePattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)

func RegisterRoutes(r *gin.Engine, h *Handler) {
	api := r.Group("/api/v1")
	api.POST("/auth/login", h.Login)
	api.POST("/webhooks/gitee/:project_key", h.HandleGiteeWebhook)
	api.POST("/webhooks/github/:project_key", h.HandleGitHubWebhook)

	secured := api.Group("")
	secured.Use(middleware.Auth(h.Config))
	secured.GET("/auth/me", h.Me)
	secured.POST("/auth/logout", h.Logout)

	secured.GET("/projects", h.ListProjects)
	secured.POST("/projects", h.CreateProject)
	secured.GET("/projects/:project_id", h.GetProject)
	secured.PATCH("/projects/:project_id", h.UpdateProject)
	secured.DELETE("/projects/:project_id", h.DeleteProject)
	secured.POST("/projects/:project_id/deploy-tasks", h.CreateProjectDeployTask)
	secured.POST("/projects/:project_id/rollback-tasks", h.CreateProjectRollbackTask)
	secured.GET("/projects/:project_id/app-logs", h.GetProjectAppLogs)
	secured.GET("/projects/:project_id/app-logs/stream", h.StreamProjectAppLogs)

	secured.GET("/deploy-tasks", h.ListDeployTasks)
	secured.GET("/deploy-tasks/:task_id", h.GetDeployTask)
	secured.GET("/deploy-tasks/:task_id/stages", h.GetDeployTaskStages)
	secured.GET("/deploy-tasks/:task_id/logs", h.GetDeployTaskLogs)
	secured.GET("/deploy-tasks/:task_id/logs/stream", h.StreamDeployTaskLogs)
	secured.POST("/deploy-tasks/:task_id/cancel", h.CancelDeployTask)
	secured.GET("/deploy-tasks/:task_id/analysis", h.AnalyzeDeployTask)

	secured.GET("/webhook-events", h.ListWebhookEvents)
	secured.GET("/webhook-events/:event_id", h.GetWebhookEvent)

	secured.GET("/dashboard/summary", h.DashboardSummary)
	secured.GET("/dashboard/recent-deploy-tasks", h.DashboardRecentDeployTasks)

	secured.GET("/settings", h.GetSettings)
	secured.PATCH("/settings", h.PatchSettings)
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid login request", err.Error())
		return
	}
	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		util.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password", nil)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		util.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password", nil)
		return
	}
	expires := time.Now().Add(h.Config.JWTDuration())
	claims := middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatUint(user.ID, 10),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.Config.JWT.Secret))
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "TOKEN_CREATE_FAILED", "Failed to create token", nil)
		return
	}
	util.OK(c, gin.H{"token": token, "expires_at": expires, "user": user})
}

func (h *Handler) Me(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	util.OK(c, gin.H{
		"id":       userID,
		"username": c.GetString(middleware.UsernameKey),
		"role":     c.GetString(middleware.RoleKey),
		"actor":    c.GetString(middleware.ActorKey),
	})
}

func (h *Handler) Logout(c *gin.Context) {
	util.OK(c, gin.H{"ok": true})
}

func (h *Handler) ListProjects(c *gin.Context) {
	page, pageSize, offset := util.ParsePagination(c)
	query := h.DB.Model(&model.Project{})
	if provider := c.Query("provider"); provider != "" {
		query = query.Where("git_provider = ?", provider)
	}
	var total int64
	_ = query.Count(&total).Error
	var projects []model.Project
	sortClause := util.SortClause(c.Query("sort"), map[string]bool{"created_at": true, "updated_at": true, "name": true}, "created_at desc")
	if err := query.Order(sortClause).Limit(pageSize).Offset(offset).Find(&projects).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "Failed to list projects", nil)
		return
	}
	util.List(c, maskProjects(projects), util.Pagination{Page: page, PageSize: pageSize, Total: total})
}

func (h *Handler) CreateProject(c *gin.Context) {
	var project model.Project
	if err := c.ShouldBindJSON(&project); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_PROJECT", "Invalid project payload", err.Error())
		return
	}
	if project.GitProvider == "" {
		project.GitProvider = model.GitProviderGitee
	}
	if project.Branch == "" {
		project.Branch = "main"
	}
	if err := validateProject(project); err != nil {
		util.Error(c, http.StatusUnprocessableEntity, "INVALID_PROJECT", err.Error(), nil)
		return
	}
	if err := h.DB.Create(&project).Error; err != nil {
		util.Error(c, http.StatusConflict, "PROJECT_CREATE_FAILED", "Failed to create project", err.Error())
		return
	}
	util.Created(c, maskProject(project))
}

func (h *Handler) GetProject(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	util.OK(c, maskProject(project))
}

func (h *Handler) UpdateProject(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_PROJECT", "Invalid project payload", err.Error())
		return
	}
	allowed := map[string]bool{
		"name": true, "project_key": true, "git_provider": true, "branch": true,
		"app_dir": true, "rollback_cmd": true, "deploy_stages": true,
		"app_log_path": true, "webhook_secret": true,
		"auto_deploy_enabled": true,
	}
	updates := map[string]interface{}{}
	stagesChanged := false
	for key, value := range payload {
		if !allowed[key] {
			continue
		}
		if key == "webhook_secret" && isMaskedValue(value) {
			continue
		}
		if err := applyProjectUpdate(&project, key, value); err != nil {
			util.Error(c, http.StatusUnprocessableEntity, "INVALID_PROJECT", err.Error(), nil)
			return
		}
		// deploy_stages is a JSON serializer field; it is persisted separately below
		// so gorm runs the serializer (map-based Updates would not).
		if key == "deploy_stages" {
			stagesChanged = true
			continue
		}
		updates[key] = projectUpdateValue(project, key)
	}
	if len(updates) == 0 && !stagesChanged {
		util.OK(c, maskProject(project))
		return
	}
	if err := validateProject(project); err != nil {
		util.Error(c, http.StatusUnprocessableEntity, "INVALID_PROJECT", err.Error(), nil)
		return
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&project).Updates(updates).Error; err != nil {
			util.Error(c, http.StatusConflict, "PROJECT_UPDATE_FAILED", "Failed to update project", err.Error())
			return
		}
	}
	if stagesChanged {
		// Select forces the field to be written even when the pipeline is emptied.
		if err := h.DB.Model(&project).Select("Stages").Updates(project).Error; err != nil {
			util.Error(c, http.StatusConflict, "PROJECT_UPDATE_FAILED", "Failed to update project", err.Error())
			return
		}
	}
	_ = h.DB.First(&project, project.ID).Error
	util.OK(c, maskProject(project))
}

func (h *Handler) DeleteProject(c *gin.Context) {
	id, ok := parseUintParam(c, "project_id")
	if !ok {
		return
	}
	if err := h.Service.DeleteProject(c.Request.Context(), id); err != nil {
		switch {
		case errors.Is(err, service.ErrProjectHasActiveTask):
			util.Error(c, http.StatusConflict, "PROJECT_HAS_ACTIVE_TASK", "Project has a pending or running deploy task", nil)
		case errors.Is(err, gorm.ErrRecordNotFound):
			util.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", nil)
		default:
			util.Error(c, http.StatusInternalServerError, "PROJECT_DELETE_FAILED", "Failed to delete project", nil)
		}
		return
	}
	util.NoContent(c)
}

func (h *Handler) CreateProjectDeployTask(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	if !h.allowMCPMutation(c) {
		return
	}
	trigger := model.TriggerManual
	if middleware.IsMCP(c) {
		trigger = model.TriggerMCP
	}
	task, err := h.Service.CreateDeployTask(c.Request.Context(), project, trigger, nil)
	if err != nil {
		h.deployTaskError(c, err)
		return
	}
	util.Accepted(c, task)
}

func (h *Handler) CreateProjectRollbackTask(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	if !h.allowMCPMutation(c) {
		return
	}
	task, err := h.Service.CreateDeployTask(c.Request.Context(), project, model.TriggerRollback, nil)
	if err != nil {
		h.deployTaskError(c, err)
		return
	}
	util.Accepted(c, task)
}

func (h *Handler) ListDeployTasks(c *gin.Context) {
	page, pageSize, offset := util.ParsePagination(c)
	query := h.DB.Model(&model.DeployTask{})
	if projectID := c.Query("project_id"); projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	_ = query.Count(&total).Error
	var tasks []model.DeployTask
	sortClause := util.SortClause(c.Query("sort"), map[string]bool{"created_at": true, "updated_at": true, "started_at": true, "finished_at": true}, "created_at desc")
	if err := query.Preload("Project").Order(sortClause).Limit(pageSize).Offset(offset).Find(&tasks).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "DEPLOY_TASK_LIST_FAILED", "Failed to list deploy tasks", nil)
		return
	}
	util.List(c, maskTasks(tasks), util.Pagination{Page: page, PageSize: pageSize, Total: total})
}

func (h *Handler) GetDeployTask(c *gin.Context) {
	id, ok := parseUintParam(c, "task_id")
	if !ok {
		return
	}
	var task model.DeployTask
	if err := h.DB.Preload("Project").Preload("Stages").First(&task, id).Error; err != nil {
		util.Error(c, http.StatusNotFound, "DEPLOY_TASK_NOT_FOUND", "Deploy task not found", nil)
		return
	}
	util.OK(c, maskTask(task))
}

func (h *Handler) GetDeployTaskStages(c *gin.Context) {
	id, ok := parseUintParam(c, "task_id")
	if !ok {
		return
	}
	var stages []model.DeployTaskStage
	if err := h.DB.Where("task_id = ?", id).Order("id asc").Find(&stages).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "STAGE_LIST_FAILED", "Failed to list task stages", nil)
		return
	}
	util.OK(c, stages)
}

func (h *Handler) GetDeployTaskLogs(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	lines := util.ParseLines(c.Query("lines"), h.Config.AppLog.MaxTailLines, h.Config.AppLog.MaxAllowedLines)
	logText, err := tailFileInDir(task.LogFile, lines, h.Config.Deploy.LogDir)
	if err != nil {
		util.Error(c, http.StatusNotFound, "LOG_NOT_FOUND", "Deploy log not found", err.Error())
		return
	}
	util.OK(c, gin.H{"task_id": task.ID, "lines": lines, "log": logText})
}

func (h *Handler) StreamDeployTaskLogs(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	if existing, err := tailFileInDir(task.LogFile, 100, h.Config.Deploy.LogDir); err == nil && existing != "" {
		for _, line := range strings.Split(strings.TrimSuffix(existing, "\n"), "\n") {
			c.SSEvent("message", line)
		}
		c.Writer.Flush()
	}
	ch, unsubscribe := h.Hub.Subscribe(sse.DeployTopic(task.ID))
	defer unsubscribe()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case line := <-ch:
			c.SSEvent("message", sanitizeLogText(strings.TrimSuffix(line, "\n")))
			return true
		}
	})
}

func (h *Handler) CancelDeployTask(c *gin.Context) {
	id, ok := parseUintParam(c, "task_id")
	if !ok {
		return
	}
	if err := h.Service.CancelTask(c.Request.Context(), id); err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			util.Error(c, http.StatusNotFound, "DEPLOY_TASK_NOT_FOUND", "Deploy task not found", nil)
			return
		}
		if errors.Is(err, service.ErrTaskNotCancelable) {
			util.Error(c, http.StatusConflict, "DEPLOY_TASK_NOT_CANCELABLE", "Deploy task is not pending or running", nil)
			return
		}
		util.Error(c, http.StatusInternalServerError, "TASK_CANCEL_FAILED", "Failed to cancel task", nil)
		return
	}
	util.Accepted(c, gin.H{"task_id": id, "status": model.TaskCanceled})
}

func (h *Handler) AnalyzeDeployTask(c *gin.Context) {
	task, ok := h.loadTask(c)
	if !ok {
		return
	}
	logText, _ := tailFileInDir(task.LogFile, 500, h.Config.Deploy.LogDir)
	util.OK(c, service.AnalyzeFailureLog(task.CurrentStage, logText))
}

func (h *Handler) GetProjectAppLogs(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	if project.AppLogPath == "" {
		util.Error(c, http.StatusUnprocessableEntity, "APP_LOG_NOT_CONFIGURED", "Project app_log_path is empty", nil)
		return
	}
	lines := util.ParseLines(c.Query("lines"), h.Config.AppLog.MaxTailLines, h.Config.AppLog.MaxAllowedLines)
	logText, err := tailProjectAppLog(project, lines)
	if err != nil {
		util.Error(c, http.StatusNotFound, "APP_LOG_NOT_FOUND", "Application log not found", err.Error())
		return
	}
	util.OK(c, gin.H{"project_id": project.ID, "lines": lines, "log": logText})
}

func (h *Handler) StreamProjectAppLogs(c *gin.Context) {
	project, ok := h.loadProject(c)
	if !ok {
		return
	}
	if project.AppLogPath == "" {
		util.Error(c, http.StatusUnprocessableEntity, "APP_LOG_NOT_CONFIGURED", "Project app_log_path is empty", nil)
		return
	}
	appLogPath, err := safeProjectAppLogPath(project)
	if err != nil {
		util.Error(c, http.StatusUnprocessableEntity, "INVALID_APP_LOG_PATH", err.Error(), nil)
		return
	}
	if _, err := os.Stat(appLogPath); err != nil {
		util.Error(c, http.StatusNotFound, "APP_LOG_NOT_FOUND", "Application log not found", err.Error())
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "tail", "-n", "100", "-F", appLogPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "APP_LOG_STREAM_FAILED", "Failed to stream application log", err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		util.Error(c, http.StatusInternalServerError, "APP_LOG_STREAM_FAILED", "Failed to stream application log", err.Error())
		return
	}
	scanner := bufio.NewScanner(stdout)
	c.Stream(func(w io.Writer) bool {
		if !scanner.Scan() {
			return false
		}
		c.SSEvent("message", sanitizeLogText(scanner.Text()))
		return true
	})
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func (h *Handler) ListWebhookEvents(c *gin.Context) {
	page, pageSize, offset := util.ParsePagination(c)
	query := h.DB.Model(&model.WebhookEvent{})
	if provider := c.Query("provider"); provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if projectID := c.Query("project_id"); projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	var total int64
	_ = query.Count(&total).Error
	var events []model.WebhookEvent
	if err := query.Order("created_at desc").Limit(pageSize).Offset(offset).Find(&events).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "WEBHOOK_EVENT_LIST_FAILED", "Failed to list webhook events", nil)
		return
	}
	util.List(c, events, util.Pagination{Page: page, PageSize: pageSize, Total: total})
}

func (h *Handler) GetWebhookEvent(c *gin.Context) {
	id, ok := parseUintParam(c, "event_id")
	if !ok {
		return
	}
	var event model.WebhookEvent
	if err := h.DB.First(&event, id).Error; err != nil {
		util.Error(c, http.StatusNotFound, "WEBHOOK_EVENT_NOT_FOUND", "Webhook event not found", nil)
		return
	}
	util.OK(c, event)
}

func (h *Handler) DashboardSummary(c *gin.Context) {
	var projectTotal int64
	var todayTotal int64
	var successTotal int64
	var failedTotal int64
	today := time.Now().Truncate(24 * time.Hour)
	h.DB.Model(&model.Project{}).Count(&projectTotal)
	h.DB.Model(&model.DeployTask{}).Where("created_at >= ?", today).Count(&todayTotal)
	h.DB.Model(&model.DeployTask{}).Where("created_at >= ? AND status IN ?", today, []string{model.TaskSuccess, model.TaskRollbacked}).Count(&successTotal)
	h.DB.Model(&model.DeployTask{}).Where("created_at >= ? AND status = ?", today, model.TaskFailed).Count(&failedTotal)
	var recentFailed []model.DeployTask
	h.DB.Where("status = ?", model.TaskFailed).Order("created_at desc").Limit(5).Find(&recentFailed)
	rate := 0.0
	if todayTotal > 0 {
		rate = float64(successTotal) / float64(todayTotal)
	}
	util.OK(c, gin.H{
		"project_total":       projectTotal,
		"today_deploy_total":  todayTotal,
		"today_success_total": successTotal,
		"today_failed_total":  failedTotal,
		"success_rate":        rate,
		"recent_failed_tasks": maskTasks(recentFailed),
	})
}

func (h *Handler) DashboardRecentDeployTasks(c *gin.Context) {
	var tasks []model.DeployTask
	limit := util.ParseLines(c.Query("limit"), 10, 50)
	if err := h.DB.Preload("Project").Order("created_at desc").Limit(limit).Find(&tasks).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "RECENT_TASKS_FAILED", "Failed to load recent deploy tasks", nil)
		return
	}
	util.OK(c, maskTasks(tasks))
}

func (h *Handler) GetSettings(c *gin.Context) {
	var settings []model.Setting
	_ = h.DB.Order("`key` asc").Find(&settings).Error
	util.OK(c, gin.H{
		"server":  gin.H{"port": h.Config.Server.Port, "cors_origins": h.Config.Server.CORSOrigins},
		"deploy":  gin.H{"log_dir": h.Config.Deploy.LogDir, "command_timeout_minutes": h.Config.Deploy.CommandTimeoutMinutes},
		"app_log": h.Config.AppLog,
		"mcp": gin.H{
			"enabled":              h.Config.MCP.Enabled,
			"allow_mutation_tools": h.Config.MCP.AllowMutationTools,
			"api_token":            util.MaskSecret(h.Config.MCP.APIToken),
		},
		"settings": settings,
	})
}

func (h *Handler) PatchSettings(c *gin.Context) {
	var payload map[string]string
	if err := c.ShouldBindJSON(&payload); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_SETTINGS", "Invalid settings payload", err.Error())
		return
	}
	for key, value := range payload {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") ||
			strings.HasPrefix(lowerKey, "server.") || strings.HasPrefix(lowerKey, "database.") ||
			strings.HasPrefix(lowerKey, "jwt.") || strings.HasPrefix(lowerKey, "deploy.") ||
			strings.HasPrefix(lowerKey, "app_log.") || strings.HasPrefix(lowerKey, "mcp.") {
			util.Error(c, http.StatusUnprocessableEntity, "SETTING_READ_ONLY", "Runtime and secret settings are managed in config.yaml", gin.H{"key": key})
			return
		}
		setting := model.Setting{Key: key, Value: value}
		if err := h.DB.Where("`key` = ?", key).Assign(setting).FirstOrCreate(&setting).Error; err != nil {
			util.Error(c, http.StatusInternalServerError, "SETTING_UPDATE_FAILED", "Failed to update setting", err.Error())
			return
		}
	}
	h.GetSettings(c)
}

func (h *Handler) HandleGiteeWebhook(c *gin.Context) {
	h.handleWebhook(c, model.GitProviderGitee, webhook.GiteeWebhookParser{})
}

func (h *Handler) HandleGitHubWebhook(c *gin.Context) {
	h.handleWebhook(c, model.GitProviderGitHub, webhook.GitHubWebhookParser{})
}

func (h *Handler) handleWebhook(c *gin.Context, provider string, parser webhook.WebhookParser) {
	projectKey := c.Param("project_key")
	body, err := c.GetRawData()
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_PAYLOAD", "Failed to read webhook payload", nil)
		return
	}
	var project model.Project
	projectErr := h.DB.Where("project_key = ?", projectKey).First(&project).Error
	ev, parseErr := parser.Parse(c.Request.Header, body)
	if parseErr != nil {
		ev = &webhook.Event{Provider: webhook.GitProvider(provider), RawPayload: body}
	}
	signatureValid := parser.VerifySignature(project.WebhookSecret, c.Request.Header, body)
	if provider == model.GitProviderGitee && project.WebhookSecret != "" {
		signatureValid = signatureValid || subtle.ConstantTimeCompare([]byte(c.Query("token")), []byte(project.WebhookSecret)) == 1
	}

	dbEvent := model.WebhookEvent{
		Provider:       provider,
		ProjectKey:     projectKey,
		EventType:      ev.EventType,
		Branch:         ev.Branch,
		CommitID:       ev.CommitID,
		CommitMessage:  ev.CommitMessage,
		CommitAuthor:   ev.CommitAuthor,
		DeliveryID:     ev.DeliveryID,
		SignatureValid: signatureValid,
		RawPayload:     datatypes.JSON(body),
	}
	if projectErr == nil {
		dbEvent.ProjectID = &project.ID
	}
	ignored := ""
	if projectErr != nil {
		ignored = "project not found"
	} else if project.GitProvider != provider {
		ignored = "project git_provider mismatch"
	} else if !signatureValid {
		ignored = "invalid webhook signature or token"
	} else if ev.EventType != "" && ev.EventType != "push" {
		ignored = "unsupported event type"
	} else if !project.AutoDeployEnabled {
		ignored = "auto deploy disabled"
	} else if ev.Branch != project.Branch {
		ignored = "branch mismatch"
	}
	if ignored != "" {
		dbEvent.IgnoredReason = ignored
		_ = h.DB.Create(&dbEvent).Error
		util.Accepted(c, gin.H{"handled": false, "ignored_reason": ignored})
		return
	}
	task, err := h.Service.CreateDeployTask(c.Request.Context(), project, model.TriggerWebhook, ev)
	if err != nil {
		dbEvent.IgnoredReason = err.Error()
		_ = h.DB.Create(&dbEvent).Error
		h.deployTaskError(c, err)
		return
	}
	dbEvent.Handled = true
	_ = h.DB.Create(&dbEvent).Error
	util.Accepted(c, gin.H{"handled": true, "task": maskTask(*task)})
}

func (h *Handler) allowMCPMutation(c *gin.Context) bool {
	if !middleware.IsMCP(c) {
		return true
	}
	if !h.Config.MCP.AllowMutationTools {
		util.Error(c, http.StatusForbidden, "MCP_MUTATION_DISABLED", "MCP mutation tools are disabled", nil)
		return false
	}
	var payload struct {
		Confirm bool `json:"confirm"`
	}
	if c.Request.Body != nil {
		raw, _ := c.GetRawData()
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		_ = json.Unmarshal(raw, &payload)
	}
	if !payload.Confirm {
		util.Error(c, http.StatusUnprocessableEntity, "CONFIRM_REQUIRED", "confirm=true is required for MCP mutation tools", nil)
		return false
	}
	return true
}

func (h *Handler) deployTaskError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrProjectBusy):
		util.Error(c, http.StatusConflict, "PROJECT_DEPLOY_RUNNING", "Project already has a running deploy task", nil)
	case errors.Is(err, service.ErrMissingRollback):
		util.Error(c, http.StatusUnprocessableEntity, "ROLLBACK_CMD_REQUIRED", "Project rollback_cmd is empty", nil)
	case errors.Is(err, service.ErrServiceShuttingDown):
		util.Error(c, http.StatusServiceUnavailable, "SERVICE_SHUTTING_DOWN", "Service is shutting down", nil)
	default:
		util.Error(c, http.StatusInternalServerError, "DEPLOY_TASK_CREATE_FAILED", "Failed to create deploy task", err.Error())
	}
}

func (h *Handler) loadProject(c *gin.Context) (model.Project, bool) {
	id, ok := parseUintParam(c, "project_id")
	if !ok {
		return model.Project{}, false
	}
	var project model.Project
	if err := h.DB.First(&project, id).Error; err != nil {
		util.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", nil)
		return model.Project{}, false
	}
	return project, true
}

func (h *Handler) loadTask(c *gin.Context) (model.DeployTask, bool) {
	id, ok := parseUintParam(c, "task_id")
	if !ok {
		return model.DeployTask{}, false
	}
	var task model.DeployTask
	if err := h.DB.First(&task, id).Error; err != nil {
		util.Error(c, http.StatusNotFound, "DEPLOY_TASK_NOT_FOUND", "Deploy task not found", nil)
		return model.DeployTask{}, false
	}
	return task, true
}

func validateProject(project model.Project) error {
	if strings.TrimSpace(project.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(project.ProjectKey) == "" {
		return fmt.Errorf("project_key is required")
	}
	if project.GitProvider != model.GitProviderGitee && project.GitProvider != model.GitProviderGitHub {
		return fmt.Errorf("git_provider must be gitee or github")
	}
	if strings.TrimSpace(project.AppDir) == "" {
		return fmt.Errorf("app_dir is required")
	}
	if !filepath.IsAbs(project.AppDir) {
		return fmt.Errorf("app_dir must be an absolute path")
	}
	if strings.TrimSpace(project.Branch) == "" {
		return fmt.Errorf("branch is required")
	}
	if project.AppLogPath != "" {
		if _, err := safeProjectAppLogPath(project); err != nil {
			return err
		}
	}
	if err := validateProjectStages(project.Stages); err != nil {
		return err
	}
	return nil
}

func parseProjectStages(value interface{}) ([]model.ProjectStage, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("deploy_stages is invalid: %v", err)
	}
	var stages []model.ProjectStage
	if err := json.Unmarshal(raw, &stages); err != nil {
		return nil, fmt.Errorf("deploy_stages must be an array of typed stage objects: %v", err)
	}
	return stages, validateProjectStages(stages)
}

func validateProjectStages(stages []model.ProjectStage) error {
	for i, st := range stages {
		if strings.TrimSpace(st.Name) == "" {
			return fmt.Errorf("deploy_stages[%d].name is required", i)
		}
		switch st.Type {
		case model.ProjectStageTypeCommand:
			var cfg model.CommandStageConfig
			if err := parseStageConfig(st, &cfg); err != nil {
				return fmt.Errorf("deploy_stages[%d].config is invalid: %v", i, err)
			}
			if st.Enabled && strings.TrimSpace(cfg.Command) == "" {
				return fmt.Errorf("deploy_stages[%d].config.command is required", i)
			}
		case model.ProjectStageTypeHealthCheck:
			var cfg model.HealthCheckStageConfig
			if err := parseStageConfig(st, &cfg); err != nil {
				return fmt.Errorf("deploy_stages[%d].config is invalid: %v", i, err)
			}
			if st.Enabled && strings.TrimSpace(cfg.URL) == "" {
				return fmt.Errorf("deploy_stages[%d].config.url is required", i)
			}
		case model.ProjectStageTypeOutboundWebhook:
			var cfg model.OutboundWebhookStageConfig
			if err := parseStageConfig(st, &cfg); err != nil {
				return fmt.Errorf("deploy_stages[%d].config is invalid: %v", i, err)
			}
			if st.Enabled && strings.TrimSpace(cfg.URL) == "" {
				return fmt.Errorf("deploy_stages[%d].config.url is required", i)
			}
			if cfg.Template != "" && cfg.Template != "dingtalk_text" && cfg.Template != "wecom_text" && cfg.Template != "feishu_text" && cfg.Template != "generic_json" {
				return fmt.Errorf("deploy_stages[%d].config.template is unsupported", i)
			}
		default:
			return fmt.Errorf("deploy_stages[%d].type must be command, health_check or outbound_webhook", i)
		}
		switch st.RunWhen {
		case "", model.ProjectStageRunWhenSuccess, model.ProjectStageRunWhenFailed, model.ProjectStageRunWhenAlways:
		default:
			return fmt.Errorf("deploy_stages[%d].run_when must be success, failed or always", i)
		}
	}
	return nil
}

func parseStageConfig(stage model.ProjectStage, out interface{}) error {
	if len(stage.Config) == 0 {
		return nil
	}
	return json.Unmarshal(stage.Config, out)
}

func applyProjectUpdate(project *model.Project, key string, value interface{}) error {
	if key == "deploy_stages" {
		stages, err := parseProjectStages(value)
		if err != nil {
			return err
		}
		if err := preserveMaskedOutboundWebhookURLs(project.Stages, stages); err != nil {
			return err
		}
		project.Stages = stages
		return nil
	}
	stringValue := ""
	if key != "auto_deploy_enabled" {
		if value != nil {
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", key)
			}
			stringValue = s
		}
	}
	switch key {
	case "name":
		project.Name = stringValue
	case "project_key":
		project.ProjectKey = stringValue
	case "git_provider":
		project.GitProvider = stringValue
	case "branch":
		project.Branch = stringValue
	case "app_dir":
		project.AppDir = stringValue
	case "rollback_cmd":
		project.RollbackCmd = stringValue
	case "app_log_path":
		project.AppLogPath = stringValue
	case "webhook_secret":
		project.WebhookSecret = stringValue
	case "auto_deploy_enabled":
		v, ok := value.(bool)
		if !ok {
			return fmt.Errorf("auto_deploy_enabled must be a boolean")
		}
		project.AutoDeployEnabled = v
	}
	return nil
}

func projectUpdateValue(project model.Project, key string) interface{} {
	switch key {
	case "name":
		return project.Name
	case "project_key":
		return project.ProjectKey
	case "git_provider":
		return project.GitProvider
	case "branch":
		return project.Branch
	case "app_dir":
		return project.AppDir
	case "rollback_cmd":
		return project.RollbackCmd
	case "deploy_stages":
		return project.Stages
	case "app_log_path":
		return project.AppLogPath
	case "webhook_secret":
		return project.WebhookSecret
	case "auto_deploy_enabled":
		return project.AutoDeployEnabled
	default:
		return nil
	}
}

func parseUintParam(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid id", gin.H{"param": name})
		return 0, false
	}
	return id, true
}

func maskProjects(projects []model.Project) []model.Project {
	out := make([]model.Project, len(projects))
	for i, project := range projects {
		out[i] = maskProject(project)
	}
	return out
}

func maskProject(project model.Project) model.Project {
	project.WebhookSecret = util.MaskSecret(project.WebhookSecret)
	for i, stage := range project.Stages {
		if stage.Type != model.ProjectStageTypeOutboundWebhook {
			continue
		}
		var cfg model.OutboundWebhookStageConfig
		if err := parseStageConfig(stage, &cfg); err != nil {
			continue
		}
		if strings.TrimSpace(cfg.URL) == "" {
			continue
		}
		cfg.URL = util.MaskSecret(cfg.URL)
		raw, err := json.Marshal(cfg)
		if err != nil {
			continue
		}
		project.Stages[i].Config = raw
	}
	return project
}

func preserveMaskedOutboundWebhookURLs(existing []model.ProjectStage, next []model.ProjectStage) error {
	used := map[int]bool{}
	for i := range next {
		if next[i].Type != model.ProjectStageTypeOutboundWebhook {
			continue
		}
		var cfg model.OutboundWebhookStageConfig
		if err := parseStageConfig(next[i], &cfg); err != nil {
			return fmt.Errorf("deploy_stages[%d].config is invalid: %v", i, err)
		}
		if !strings.Contains(cfg.URL, "******") {
			continue
		}
		oldCfg, ok := findExistingOutboundWebhookConfig(existing, next[i], i, used)
		if !ok || strings.TrimSpace(oldCfg.URL) == "" {
			continue
		}
		cfg.URL = oldCfg.URL
		raw, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("deploy_stages[%d].config is invalid: %v", i, err)
		}
		next[i].Config = raw
	}
	return nil
}

func findExistingOutboundWebhookConfig(stages []model.ProjectStage, target model.ProjectStage, index int, used map[int]bool) (model.OutboundWebhookStageConfig, bool) {
	for i, stage := range stages {
		if used[i] || stage.Type != model.ProjectStageTypeOutboundWebhook || stage.Name != target.Name {
			continue
		}
		cfg, ok := outboundWebhookConfig(stage)
		if ok {
			used[i] = true
			return cfg, true
		}
	}
	if index >= 0 && index < len(stages) && !used[index] {
		stage := stages[index]
		if stage.Type == model.ProjectStageTypeOutboundWebhook {
			cfg, ok := outboundWebhookConfig(stage)
			if ok {
				used[index] = true
				return cfg, true
			}
		}
	}
	return model.OutboundWebhookStageConfig{}, false
}

func outboundWebhookConfig(stage model.ProjectStage) (model.OutboundWebhookStageConfig, bool) {
	var cfg model.OutboundWebhookStageConfig
	if err := parseStageConfig(stage, &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}

func maskTasks(tasks []model.DeployTask) []model.DeployTask {
	out := make([]model.DeployTask, len(tasks))
	for i, task := range tasks {
		out[i] = maskTask(task)
	}
	return out
}

func maskTask(task model.DeployTask) model.DeployTask {
	if task.Project != nil {
		masked := maskProject(*task.Project)
		task.Project = &masked
	}
	return task
}

func isMaskedValue(value interface{}) bool {
	s, ok := value.(string)
	return ok && strings.Contains(s, "******")
}

func tailProjectAppLog(project model.Project, lines int) (string, error) {
	path, err := safeProjectAppLogPath(project)
	if err != nil {
		return "", err
	}
	return tailFile(path, lines)
}

func tailFileInDir(path string, lines int, baseDir string) (string, error) {
	safePath, err := safePathInDir(path, baseDir)
	if err != nil {
		return "", err
	}
	return tailFile(safePath, lines)
}

func tailFile(path string, lines int) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("log path must be absolute")
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	cmd := exec.Command("tail", "-n", strconv.Itoa(lines), path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return sanitizeLogText(string(out)), nil
}

func sanitizeLogText(text string) string {
	return strings.ToValidUTF8(ansiEscapePattern.ReplaceAllString(text, ""), "�")
}

func safeProjectAppLogPath(project model.Project) (string, error) {
	if project.AppLogPath == "" {
		return "", fmt.Errorf("app_log_path is empty")
	}
	return safePathInDir(project.AppLogPath, project.AppDir)
}

func safePathInDir(path string, baseDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(baseDir) {
		return "", fmt.Errorf("path and base directory must be absolute")
	}
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(baseDir)
	rel, err := filepath.Rel(cleanBase, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path must stay under %s", cleanBase)
	}
	if resolvedPath, err := filepath.EvalSymlinks(cleanPath); err == nil {
		if resolvedBase, baseErr := filepath.EvalSymlinks(cleanBase); baseErr == nil {
			rel, err := filepath.Rel(resolvedBase, resolvedPath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return "", fmt.Errorf("resolved path must stay under %s", resolvedBase)
			}
			return resolvedPath, nil
		}
	}
	return cleanPath, nil
}
