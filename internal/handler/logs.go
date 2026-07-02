package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/sse"
	"github.com/hellodeveye/postdare-go/internal/util"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)

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
