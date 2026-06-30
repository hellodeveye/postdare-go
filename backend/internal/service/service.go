package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/model"
	"postdare-go/backend/internal/notifier"
	"postdare-go/backend/internal/runner"
	"postdare-go/backend/internal/sse"
	"postdare-go/backend/internal/webhook"
)

var (
	ErrProjectBusy         = errors.New("project has a running deploy task")
	ErrMissingRollback     = errors.New("project rollback_cmd is empty")
	ErrMutationDisabled    = errors.New("mcp mutation tools are disabled")
	ErrTaskNotFound        = errors.New("deploy task not found")
	ErrTaskNotCancelable   = errors.New("deploy task is not pending or running")
	ErrServiceShuttingDown = errors.New("service is shutting down")
)

type Service struct {
	DB       *gorm.DB
	Config   *config.Config
	Runner   runner.CommandRunner
	Hub      *sse.Hub
	Notifier *notifier.Notifier
	Logger   *zap.Logger

	cancelMu     sync.Mutex
	cancels      map[uint64]context.CancelFunc
	shuttingDown bool
	taskWG       sync.WaitGroup
}

func New(database *gorm.DB, cfg *config.Config, hub *sse.Hub, logger *zap.Logger) *Service {
	return &Service{
		DB:     database,
		Config: cfg,
		Runner: &runner.LocalCommandRunner{
			LogDir:  cfg.Deploy.LogDir,
			Timeout: cfg.CommandTimeout(),
			Hub:     hub,
			Logger:  logger,
		},
		Hub:      hub,
		Notifier: notifier.New(logger),
		Logger:   logger,
		cancels:  map[uint64]context.CancelFunc{},
	}
}

func (s *Service) CreateDeployTask(ctx context.Context, project model.Project, triggerType string, ev *webhook.Event) (*model.DeployTask, error) {
	if s.isShuttingDown() {
		return nil, ErrServiceShuttingDown
	}
	if triggerType == model.TriggerRollback && strings.TrimSpace(project.RollbackCmd) == "" {
		return nil, ErrMissingRollback
	}
	var task *model.DeployTask
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedProject model.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProject, project.ID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.DeployTask{}).
			Where("project_id = ? AND status IN ?", project.ID, []string{model.TaskPending, model.TaskRunning}).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrProjectBusy
		}
		task = &model.DeployTask{
			ProjectID:   project.ID,
			TriggerType: triggerType,
			GitProvider: project.GitProvider,
			Branch:      project.Branch,
			Status:      model.TaskPending,
		}
		if ev != nil {
			task.GitProvider = string(ev.Provider)
			task.Branch = ev.Branch
			task.CommitID = ev.CommitID
			task.CommitMessage = ev.CommitMessage
			task.CommitAuthor = ev.CommitAuthor
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		task.LogFile = filepath.Join(s.Config.Deploy.LogDir, fmt.Sprintf("%d.log", task.ID))
		if err := tx.Model(task).Update("log_file", task.LogFile).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.startTask(task.ID); err != nil {
		now := time.Now()
		_ = s.DB.WithContext(ctx).Model(task).Updates(map[string]interface{}{
			"status":      model.TaskFailed,
			"fail_reason": err.Error(),
			"finished_at": now,
		}).Error
		return nil, err
	}
	return task, nil
}

func (s *Service) CancelTask(ctx context.Context, taskID uint64) error {
	var task model.DeployTask
	if err := s.DB.WithContext(ctx).Select("id", "status").First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return err
	}
	if task.Status != model.TaskPending && task.Status != model.TaskRunning {
		return ErrTaskNotCancelable
	}
	s.cancelTask(taskID)
	result := s.DB.WithContext(ctx).Model(&model.DeployTask{}).
		Where("id = ? AND status IN ?", taskID, []string{model.TaskPending, model.TaskRunning}).
		Updates(map[string]interface{}{
			"status":      model.TaskCanceled,
			"fail_reason": "canceled by user",
			"finished_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotCancelable
	}
	return nil
}

func (s *Service) ReconcileInterruptedTasks() error {
	now := time.Now()
	return s.DB.Model(&model.DeployTask{}).
		Where("status IN ?", []string{model.TaskPending, model.TaskRunning}).
		Updates(map[string]interface{}{
			"status":      model.TaskFailed,
			"fail_reason": "task interrupted by server restart",
			"finished_at": now,
		}).Error
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.cancelMu.Lock()
	s.shuttingDown = true
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.cancelMu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		s.taskWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) ExecuteTask(ctx context.Context, taskID uint64) {
	var task model.DeployTask
	if err := s.DB.WithContext(ctx).First(&task, taskID).Error; err != nil {
		s.Logger.Error("load task failed", zap.Uint64("task_id", taskID), zap.Error(err))
		return
	}
	if task.Status == model.TaskCanceled {
		return
	}
	var project model.Project
	if err := s.DB.WithContext(ctx).First(&project, task.ProjectID).Error; err != nil {
		s.failTask(ctx, &task, "load_project", err)
		return
	}
	started := time.Now()
	task.StartedAt = &started
	task.Status = model.TaskRunning
	task.LogFile = filepath.Join(s.Config.Deploy.LogDir, fmt.Sprintf("%d.log", task.ID))
	_ = os.MkdirAll(s.Config.Deploy.LogDir, 0o755)
	_ = s.DB.WithContext(ctx).Save(&task).Error
	runner.AppendLog(task.LogFile, s.Hub, task.ID, "task", "task started")

	if task.TriggerType == model.TriggerRollback {
		s.executeRollback(ctx, project, &task)
		return
	}
	s.executeDeploy(ctx, project, &task)
}

func (s *Service) executeDeploy(ctx context.Context, project model.Project, task *model.DeployTask) {
	stages := []struct {
		name    string
		command string
	}{
		{name: "pull_code", command: pullCommand(project)},
		{name: "unit_test", command: project.UnitTestCmd},
		{name: "integration_test", command: project.IntegrationTestCmd},
		{name: "build", command: project.BuildCmd},
		{name: "deploy", command: project.DeployCmd},
	}
	for _, stage := range stages {
		if !s.executeCommandStage(ctx, task, stage.name, stage.command) {
			if task.Status != model.TaskCanceled {
				s.notify(ctx, project, *task, "部署失败")
			}
			return
		}
	}
	if !s.executeHealthCheckStage(ctx, task, project.HealthURL) {
		if task.Status != model.TaskCanceled {
			s.notify(ctx, project, *task, "健康检查失败")
		}
		return
	}
	s.executeNotifyStage(ctx, project, *task, "部署成功")
	s.finishTask(ctx, task, model.TaskSuccess, "")
}

func (s *Service) executeRollback(ctx context.Context, project model.Project, task *model.DeployTask) {
	if !s.executeCommandStage(ctx, task, "rollback", project.RollbackCmd) {
		if task.Status != model.TaskCanceled {
			s.notify(ctx, project, *task, "回滚失败")
		}
		return
	}
	if !s.executeHealthCheckStage(ctx, task, project.HealthURL) {
		if task.Status != model.TaskCanceled {
			s.notify(ctx, project, *task, "健康检查失败")
		}
		return
	}
	s.executeNotifyStage(ctx, project, *task, "回滚成功")
	s.finishTask(ctx, task, model.TaskRollbacked, "")
}

func (s *Service) executeCommandStage(ctx context.Context, task *model.DeployTask, name string, command string) bool {
	if s.isCanceled(ctx, task.ID) {
		s.finishTask(ctx, task, model.TaskCanceled, "canceled by user")
		return false
	}
	stage := s.startStage(ctx, task, name)
	if strings.TrimSpace(command) == "" {
		now := time.Now()
		stage.Status = model.StageSkipped
		stage.FinishedAt = &now
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "stage skipped: empty command")
		return true
	}
	runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "stage started")
	err := s.Runner.Run(ctx, task.ID, name, command)
	now := time.Now()
	stage.FinishedAt = &now
	if err != nil {
		exit := 1
		stage.ExitCode = &exit
		stage.Status = model.StageFailed
		stage.ErrorMessage = err.Error()
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		if s.isCanceled(context.Background(), task.ID) {
			s.finishTask(context.Background(), task, model.TaskCanceled, "canceled by user")
			return false
		}
		s.failTask(ctx, task, name, err)
		return false
	}
	exit := 0
	stage.ExitCode = &exit
	stage.Status = model.StageSuccess
	_ = s.DB.WithContext(ctx).Save(&stage).Error
	return true
}

func (s *Service) executeHealthCheckStage(ctx context.Context, task *model.DeployTask, healthURL string) bool {
	stage := s.startStage(ctx, task, "health_check")
	if strings.TrimSpace(healthURL) == "" {
		now := time.Now()
		stage.Status = model.StageSkipped
		stage.FinishedAt = &now
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, "health_check", "stage skipped: empty health_url")
		return true
	}
	if s.finishHealthCheckIfCanceled(ctx, task, &stage) {
		return false
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for i := 1; i <= 5; i++ {
		if s.finishHealthCheckIfCanceled(ctx, task, &stage) {
			return false
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			lastErr = err
			break
		}
		resp, err := client.Do(req)
		if s.healthCheckCanceled(ctx, task.ID, err) {
			s.finishCanceledHealthCheck(task, &stage)
			return false
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
				runner.AppendLog(task.LogFile, s.Hub, task.ID, "health_check", fmt.Sprintf("health check passed on attempt %d", i))
				return true
			}
			lastErr = fmt.Errorf("health check returned status %d", resp.StatusCode)
		} else if err != nil {
			lastErr = err
		}
		runner.AppendLog(task.LogFile, s.Hub, task.ID, "health_check", fmt.Sprintf("attempt %d failed: %v", i, lastErr))
		if i < 5 {
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				s.finishCanceledHealthCheck(task, &stage)
				return false
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
	s.failTask(ctx, task, "health_check", fmt.Errorf("health check failed: %w", lastErr))
	return false
}

func (s *Service) healthCheckCanceled(ctx context.Context, taskID uint64, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return true
	}
	return s.isCanceled(context.Background(), taskID)
}

func (s *Service) finishHealthCheckIfCanceled(ctx context.Context, task *model.DeployTask, stage *model.DeployTaskStage) bool {
	if !s.healthCheckCanceled(ctx, task.ID, nil) {
		return false
	}
	s.finishCanceledHealthCheck(task, stage)
	return true
}

func (s *Service) finishCanceledHealthCheck(task *model.DeployTask, stage *model.DeployTaskStage) {
	now := time.Now()
	exit := 1
	stage.Status = model.StageFailed
	stage.FinishedAt = &now
	stage.ExitCode = &exit
	stage.ErrorMessage = "canceled by user"
	_ = s.DB.WithContext(context.Background()).Save(stage).Error
	s.finishTask(context.Background(), task, model.TaskCanceled, "canceled by user")
}

func (s *Service) executeNotifyStage(ctx context.Context, project model.Project, task model.DeployTask, scene string) {
	stage := s.startStage(ctx, &task, "notify")
	err := s.Notifier.Send(project, task, scene)
	now := time.Now()
	stage.FinishedAt = &now
	stage.Status = model.StageSuccess
	exit := 0
	stage.ExitCode = &exit
	if err != nil {
		stage.ErrorMessage = err.Error()
		runner.AppendLog(task.LogFile, s.Hub, task.ID, "notify", "notification failed: "+err.Error())
	} else {
		runner.AppendLog(task.LogFile, s.Hub, task.ID, "notify", "notification sent or not configured")
	}
	_ = s.DB.WithContext(ctx).Save(&stage).Error
}

func (s *Service) notify(ctx context.Context, project model.Project, task model.DeployTask, scene string) {
	err := s.Notifier.Send(project, task, scene)
	if err != nil {
		runner.AppendLog(task.LogFile, s.Hub, task.ID, "notify", "notification failed: "+err.Error())
	}
}

func (s *Service) startStage(ctx context.Context, task *model.DeployTask, name string) model.DeployTaskStage {
	now := time.Now()
	task.CurrentStage = name
	_ = s.DB.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"current_stage": name,
		"status":        model.TaskRunning,
	}).Error
	stage := model.DeployTaskStage{
		TaskID:    task.ID,
		Name:      name,
		Status:    model.StageRunning,
		StartedAt: &now,
	}
	_ = s.DB.WithContext(ctx).Create(&stage).Error
	return stage
}

func (s *Service) failTask(ctx context.Context, task *model.DeployTask, stage string, err error) {
	s.finishTask(ctx, task, model.TaskFailed, err.Error())
	task.CurrentStage = stage
	runner.AppendLog(task.LogFile, s.Hub, task.ID, stage, "task failed: "+err.Error())
}

func (s *Service) finishTask(ctx context.Context, task *model.DeployTask, status string, failReason string) {
	now := time.Now()
	task.Status = status
	task.FailReason = failReason
	task.FinishedAt = &now
	_ = s.DB.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"status":        status,
		"fail_reason":   failReason,
		"finished_at":   now,
		"current_stage": task.CurrentStage,
	}).Error
	runner.AppendLog(task.LogFile, s.Hub, task.ID, "task", "task finished with status "+status)
}

func (s *Service) ensureNoRunningTask(projectID uint64) error {
	var count int64
	err := s.DB.Model(&model.DeployTask{}).
		Where("project_id = ? AND status IN ?", projectID, []string{model.TaskPending, model.TaskRunning}).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProjectBusy
	}
	return nil
}

func (s *Service) unregisterCancel(taskID uint64) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	delete(s.cancels, taskID)
}

func (s *Service) startTask(taskID uint64) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	if s.shuttingDown {
		s.cancelMu.Unlock()
		cancel()
		return ErrServiceShuttingDown
	}
	s.cancels[taskID] = cancel
	s.taskWG.Add(1)
	s.cancelMu.Unlock()

	go func() {
		defer s.taskWG.Done()
		defer s.unregisterCancel(taskID)
		defer cancel()
		s.ExecuteTask(ctx, taskID)
	}()
	return nil
}

func (s *Service) cancelTask(taskID uint64) {
	s.cancelMu.Lock()
	cancel := s.cancels[taskID]
	s.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) isShuttingDown() bool {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.shuttingDown
}

func (s *Service) isCanceled(ctx context.Context, taskID uint64) bool {
	var task model.DeployTask
	if err := s.DB.WithContext(ctx).Select("status").First(&task, taskID).Error; err != nil {
		return false
	}
	return task.Status == model.TaskCanceled
}

func pullCommand(project model.Project) string {
	if strings.TrimSpace(project.PullCmd) != "" {
		return project.PullCmd
	}
	return fmt.Sprintf("cd %s && git fetch --all && git reset --hard origin/%s", shellQuote(project.RepoDir), shellQuote(project.Branch))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
