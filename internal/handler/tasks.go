package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hellodeveye/postdare-go/internal/middleware"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/service"
	"github.com/hellodeveye/postdare-go/internal/util"
)

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
