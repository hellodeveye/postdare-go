package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/util"
	"gorm.io/gorm/clause"
)

func (h *Handler) GetSettings(c *gin.Context) {
	var settings []model.Setting
	_ = h.DB.Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).Find(&settings).Error
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
