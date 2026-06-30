package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
)

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

	database, err := db.Open(cfg.Database)
	if err != nil {
		logger.Fatal("database init failed", zap.Error(err))
	}

	hub := sse.NewHub()
	svc := service.New(database, cfg, hub, logger)
	if err := svc.ReconcileInterruptedTasks(); err != nil {
		logger.Fatal("task reconciliation failed", zap.Error(err))
	}
	h := &handler.Handler{DB: database, Config: cfg, Service: svc, Hub: hub}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.Logger(logger), middleware.CORS(cfg.Server.CORSOrigins))
	handler.RegisterRoutes(router, h)

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
