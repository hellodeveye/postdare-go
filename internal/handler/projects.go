package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/service"
	"github.com/hellodeveye/postdare-go/internal/util"
	"gorm.io/gorm"
)

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
