package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/db"
	"postdare-go/backend/internal/handler"
	"postdare-go/backend/internal/middleware"
	"postdare-go/backend/internal/service"
	"postdare-go/backend/internal/sse"
	"postdare-go/backend/internal/util"
	"postdare-go/backend/internal/webui"
)

var version = "dev"

func main() {
	cfgPath := os.Getenv("POSTDARE_GO_CONFIG")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Info("startup configuration",
		zap.String("db_driver", cfg.Database.Driver),
		zap.String("db_path", cfg.Database.Path),
		zap.Bool("db_dsn_configured", cfg.Database.DSN != ""),
		zap.String("deploy_log_dir", cfg.Deploy.LogDir),
		zap.Int("port", cfg.Server.Port),
		zap.Bool("mcp_enabled", cfg.MCP.Enabled),
		zap.Bool("mcp_mutation_tools", cfg.MCP.AllowMutationTools),
		zap.Bool("jwt_secret_configured", cfg.JWT.Secret != ""),
		zap.Bool("mcp_api_token_configured", cfg.MCP.APIToken != ""),
	)

	database, err := db.Open(
		cfg.Database,
		db.WithAdminPassword(cfg.AdminPassword),
		db.WithGeneratedPasswordLogger(func(password string) {
			logger.Warn("generated initial admin password",
				zap.String("username", "admin"),
				zap.String("password", password),
			)
		}),
	)
	if err != nil {
		logger.Fatal("database init failed", zap.Error(err))
	}

	hub := sse.NewHub()
	svc := service.New(database, cfg, hub, logger)
	if err := svc.ReconcileInterruptedTasks(); err != nil {
		logger.Fatal("task reconciliation failed", zap.Error(err))
	}
	h := &handler.Handler{DB: database, Config: cfg, Service: svc, Hub: hub, AppVersion: version}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.Logger(logger), middleware.CORS(cfg.Server.CORSOrigins))
	handler.RegisterRoutes(router, h)
	registerWebUI(router)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{Addr: addr, Handler: router}
	logger.Info("Postdare Go backend listening", zap.String("addr", addr))
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server stopped", zap.Error(err))
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown failed", zap.Error(err))
	}
	if err := svc.Shutdown(shutdownCtx); err != nil {
		logger.Warn("task shutdown timed out", zap.Error(err))
	}
}

func registerWebUI(router *gin.Engine) {
	dist, err := webui.Dist()
	if err != nil {
		router.NoRoute(func(c *gin.Context) {
			if isAPIPath(c.Request.URL.Path) {
				util.Error(c, http.StatusNotFound, "NOT_FOUND", "API endpoint not found", nil)
				return
			}
			util.Error(c, http.StatusInternalServerError, "WEB_UI_UNAVAILABLE", "Embedded web UI is unavailable", err.Error())
		})
		return
	}
	router.NoRoute(func(c *gin.Context) {
		if isAPIPath(c.Request.URL.Path) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "API endpoint not found", nil)
			return
		}
		requestPath := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if requestPath == "" || requestPath == "." {
			requestPath = "index.html"
		}
		if fileExists(dist, requestPath) {
			setWebUICacheHeaders(c, requestPath)
			serveWebUIFile(c, dist, requestPath)
			return
		}
		if !fileExists(dist, "index.html") {
			serveWebUIPlaceholder(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		serveWebUIFile(c, dist, "index.html")
	})
}

func isAPIPath(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/")
}

func fileExists(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}

func setWebUICacheHeaders(c *gin.Context, requestPath string) {
	if strings.HasPrefix(requestPath, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	if requestPath == "index.html" {
		c.Header("Cache-Control", "no-cache")
	}
}

func serveWebUIFile(c *gin.Context, fsys fs.FS, name string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		util.Error(c, http.StatusNotFound, "WEB_UI_FILE_NOT_FOUND", "Embedded web UI file not found", nil)
		return
	}
	info, err := fs.Stat(fsys, name)
	if err != nil {
		util.Error(c, http.StatusNotFound, "WEB_UI_FILE_NOT_FOUND", "Embedded web UI file not found", nil)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	c.Header("Content-Type", contentType)
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), bytes.NewReader(data))
}

func serveWebUIPlaceholder(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Postdare Go</title>
    <style>
      :root{color-scheme:dark;background:#0d1117;color:#e6edf3;font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
      body{min-height:100vh;margin:0;display:grid;place-items:center}
      main{width:min(420px,calc(100vw - 32px));border:1px solid #30363d;border-radius:8px;background:#161b22;padding:24px}
      h1{margin:0 0 8px;font-size:20px}p{margin:0;color:#8b949e;line-height:1.6}code{color:#79c0ff}
    </style>
  </head>
  <body><main><h1>Web UI not embedded</h1><p>Run <code>make web</code> or <code>make release</code>, then rebuild the server binary.</p></main></body>
</html>`))
}
