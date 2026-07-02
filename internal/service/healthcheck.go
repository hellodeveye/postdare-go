package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/runner"
)

func (s *Service) runHealthCheckStage(ctx context.Context, task *model.DeployTask, name string, healthURL string) (stageOutcome, error) {
	if s.isCanceled(ctx, task.ID) {
		return stageCanceled, nil
	}
	stage := s.startStage(ctx, task, name)
	if strings.TrimSpace(healthURL) == "" {
		now := time.Now()
		stage.Status = model.StageSkipped
		stage.FinishedAt = &now
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "stage skipped: empty health check url")
		return stageOK, nil
	}
	if s.markHealthCheckCanceled(ctx, task, &stage) {
		return stageCanceled, nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for i := 1; i <= 5; i++ {
		if s.markHealthCheckCanceled(ctx, task, &stage) {
			return stageCanceled, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			lastErr = err
			break
		}
		resp, err := client.Do(req)
		if s.healthCheckCanceled(ctx, task.ID, err) {
			s.markCanceledStage(&stage)
			return stageCanceled, nil
		}
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				now := time.Now()
				stage.Status = model.StageSuccess
				stage.FinishedAt = &now
				exit := 0
				stage.ExitCode = &exit
				_ = s.DB.WithContext(ctx).Save(&stage).Error
				runner.AppendLog(task.LogFile, s.Hub, task.ID, name, fmt.Sprintf("health check passed on attempt %d", i))
				return stageOK, nil
			}
			lastErr = fmt.Errorf("health check returned status %d", resp.StatusCode)
		} else if err != nil {
			lastErr = err
		}
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, fmt.Sprintf("attempt %d failed: %v", i, lastErr))
		if i < 5 {
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				s.markCanceledStage(&stage)
				return stageCanceled, nil
			case <-timer.C:
			}
		}
	}
	now := time.Now()
	stage.Status = model.StageFailed
	stage.FinishedAt = &now
	exit := 1
	stage.ExitCode = &exit
	stage.ErrorMessage = fmt.Sprintf("health check failed: %v", lastErr)
	_ = s.DB.WithContext(ctx).Save(&stage).Error
	return stageFailed, fmt.Errorf("health check failed: %w", lastErr)
}

func (s *Service) healthCheckCanceled(ctx context.Context, taskID uint64, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return true
	}
	return s.isCanceled(context.Background(), taskID)
}

func (s *Service) markHealthCheckCanceled(ctx context.Context, task *model.DeployTask, stage *model.DeployTaskStage) bool {
	if !s.healthCheckCanceled(ctx, task.ID, nil) {
		return false
	}
	s.markCanceledStage(stage)
	return true
}

func (s *Service) markCanceledStage(stage *model.DeployTaskStage) {
	now := time.Now()
	exit := 1
	stage.Status = model.StageFailed
	stage.FinishedAt = &now
	stage.ExitCode = &exit
	stage.ErrorMessage = "canceled by user"
	_ = s.DB.WithContext(context.Background()).Save(stage).Error
}
