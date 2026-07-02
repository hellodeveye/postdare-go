package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/util"
)

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
