package handler

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetVersion(c *gin.Context) {
	version := h.AppVersion
	if version == "" {
		version = "dev"
	}
	c.JSON(http.StatusOK, gin.H{"version": version, "go": runtime.Version()})
}
