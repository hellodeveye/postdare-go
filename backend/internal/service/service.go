package service

import (
	"context"
	"encoding/json"
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
	for _, stage := range DeployStages(project) {
		if !shouldRunInMainFlow(stage) {
			continue
		}
		outcome, err := s.runProjectStage(ctx, project, task, stage)
		switch outcome {
		case stageOK:
			continue
		case stageCanceled:
			s.finishTask(context.Background(), task, model.TaskCanceled, "canceled by user")
			return
		default: // stageFailed
			if stage.ContinueOnError {
				runner.AppendLog(task.LogFile, s.Hub, task.ID, stage.Name, fmt.Sprintf("stage failed but continue_on_error is set, continuing: %v", err))
				continue
			}
			s.failTask(ctx, task, stage.Name, err)
			s.executeDeferredStages(context.Background(), project, task)
			return
		}
	}
	s.finishTask(ctx, task, model.TaskSuccess, "")
	s.executeDeferredStages(context.Background(), project, task)
}

func (s *Service) executeRollback(ctx context.Context, project model.Project, task *model.DeployTask) {
	if !s.executeCommandStage(ctx, task, "rollback", project.RollbackCmd) {
		if task.Status != model.TaskCanceled {
			s.executeDeferredStages(context.Background(), project, task)
		}
		return
	}
	s.finishTask(ctx, task, model.TaskRollbacked, "")
	s.executeDeferredStages(context.Background(), project, task)
}

// stageOutcome is the result of running a single command stage without any
// task-level side effects. Callers decide how a failure affects the task
// (deploy honors continue_on_error, rollback always aborts).
type stageOutcome int

const (
	stageOK stageOutcome = iota
	stageFailed
	stageCanceled
)

// runCommandStage executes one shell-command stage and records its DeployTaskStage row.
// It never mutates task-level status; on failure it returns the underlying error
// so the caller can surface it via failTask when appropriate.
func (s *Service) runCommandStage(ctx context.Context, task *model.DeployTask, name string, command string) (stageOutcome, error) {
	if s.isCanceled(ctx, task.ID) {
		return stageCanceled, nil
	}
	stage := s.startStage(ctx, task, name)
	if strings.TrimSpace(command) == "" {
		now := time.Now()
		stage.Status = model.StageSkipped
		stage.FinishedAt = &now
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "stage skipped: empty command")
		return stageOK, nil
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
			return stageCanceled, err
		}
		return stageFailed, err
	}
	exit := 0
	stage.ExitCode = &exit
	stage.Status = model.StageSuccess
	_ = s.DB.WithContext(ctx).Save(&stage).Error
	return stageOK, nil
}

// executeCommandStage runs a stage and applies the default "fail the whole task on
// error" behavior. Used by the rollback path, which has no per-stage error policy.
func (s *Service) executeCommandStage(ctx context.Context, task *model.DeployTask, name string, command string) bool {
	switch outcome, err := s.runCommandStage(ctx, task, name, command); outcome {
	case stageOK:
		return true
	case stageCanceled:
		s.finishTask(context.Background(), task, model.TaskCanceled, "canceled by user")
		return false
	default: // stageFailed
		s.failTask(ctx, task, name, err)
		return false
	}
}

// DeployStages returns the ordered stages configured for a deploy.
func DeployStages(project model.Project) []model.ProjectStage {
	return project.Stages
}

func shouldRunInMainFlow(stage model.ProjectStage) bool {
	return stage.Enabled && strings.TrimSpace(stage.RunWhen) == ""
}

func shouldRunDeferred(stage model.ProjectStage, terminalStatus string) bool {
	if !stage.Enabled {
		return false
	}
	switch stage.RunWhen {
	case model.ProjectStageRunWhenAlways:
		return true
	case model.ProjectStageRunWhenSuccess:
		return terminalStatus == model.TaskSuccess || terminalStatus == model.TaskRollbacked
	case model.ProjectStageRunWhenFailed:
		return terminalStatus == model.TaskFailed
	default:
		return false
	}
}

func (s *Service) executeDeferredStages(ctx context.Context, project model.Project, task *model.DeployTask) {
	for _, stage := range DeployStages(project) {
		if !shouldRunDeferred(stage, task.Status) {
			continue
		}
		outcome, err := s.runProjectStage(ctx, project, task, stage)
		if outcome == stageCanceled {
			return
		}
		if err != nil {
			runner.AppendLog(task.LogFile, s.Hub, task.ID, stage.Name, fmt.Sprintf("deferred stage failed: %v", err))
		}
	}
}

func (s *Service) runProjectStage(ctx context.Context, project model.Project, task *model.DeployTask, stage model.ProjectStage) (stageOutcome, error) {
	switch stage.Type {
	case model.ProjectStageTypeCommand:
		cfg, err := commandStageConfig(stage)
		if err != nil {
			return stageFailed, err
		}
		return s.runCommandStage(ctx, task, stage.Name, cfg.Command)
	case model.ProjectStageTypeHealthCheck:
		cfg, err := healthCheckStageConfig(stage)
		if err != nil {
			return stageFailed, err
		}
		return s.runHealthCheckStage(ctx, task, stage.Name, strings.TrimSpace(cfg.URL))
	case model.ProjectStageTypeOutboundWebhook:
		cfg, err := outboundWebhookStageConfig(stage)
		if err != nil {
			return stageFailed, err
		}
		return s.runOutboundWebhookStage(ctx, project, task, stage.Name, cfg)
	default:
		return stageFailed, fmt.Errorf("unsupported stage type %q", stage.Type)
	}
}

func commandStageConfig(stage model.ProjectStage) (model.CommandStageConfig, error) {
	var cfg model.CommandStageConfig
	if err := decodeStageConfig(stage, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func healthCheckStageConfig(stage model.ProjectStage) (model.HealthCheckStageConfig, error) {
	var cfg model.HealthCheckStageConfig
	if err := decodeStageConfig(stage, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func outboundWebhookStageConfig(stage model.ProjectStage) (model.OutboundWebhookStageConfig, error) {
	var cfg model.OutboundWebhookStageConfig
	if err := decodeStageConfig(stage, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func decodeStageConfig(stage model.ProjectStage, out interface{}) error {
	if len(stage.Config) == 0 {
		return nil
	}
	if err := json.Unmarshal(stage.Config, out); err != nil {
		return fmt.Errorf("invalid config for stage %s: %w", stage.Name, err)
	}
	return nil
}

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

func (s *Service) runOutboundWebhookStage(ctx context.Context, project model.Project, task *model.DeployTask, name string, cfg model.OutboundWebhookStageConfig) (stageOutcome, error) {
	if s.isCanceled(ctx, task.ID) {
		return stageCanceled, nil
	}
	stage := s.startStage(ctx, task, name)
	webhookURL := strings.TrimSpace(cfg.URL)
	if webhookURL == "" {
		now := time.Now()
		stage.Status = model.StageSkipped
		stage.FinishedAt = &now
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "stage skipped: outbound webhook url is empty")
		return stageOK, nil
	}
	cfg.URL = webhookURL
	err := s.Notifier.SendOutboundWebhook(project, *task, cfg)
	now := time.Now()
	stage.FinishedAt = &now
	if err != nil {
		exit := 1
		stage.ExitCode = &exit
		stage.Status = model.StageFailed
		stage.ErrorMessage = err.Error()
		_ = s.DB.WithContext(ctx).Save(&stage).Error
		runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "outbound webhook failed: "+err.Error())
		return stageFailed, err
	}
	exit := 0
	stage.ExitCode = &exit
	stage.Status = model.StageSuccess
	_ = s.DB.WithContext(ctx).Save(&stage).Error
	runner.AppendLog(task.LogFile, s.Hub, task.ID, name, "outbound webhook sent")
	return stageOK, nil
}

func (s *Service) startStage(ctx context.Context, task *model.DeployTask, name string) model.DeployTaskStage {
	now := time.Now()
	task.CurrentStage = name
	_ = s.DB.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"current_stage": name,
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
