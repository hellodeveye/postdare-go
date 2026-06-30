package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"postdare-go/backend/internal/model"
)

type Notifier struct {
	Client *http.Client
	Logger *zap.Logger
}

func New(logger *zap.Logger) *Notifier {
	return &Notifier{
		Client: &http.Client{Timeout: 10 * time.Second},
		Logger: logger,
	}
}

func (n *Notifier) Send(project model.Project, task model.DeployTask, scene string) error {
	if project.NotifyWebhook == "" {
		return nil
	}
	content := fmt.Sprintf("Postdare Go %s\n项目: %s\nGit: %s\n触发: %s\n分支: %s\ncommit: %s\n消息: %s\n阶段: %s\n状态: %s\n失败原因: %s\n任务ID: %d",
		scene,
		project.Name,
		task.GitProvider,
		task.TriggerType,
		task.Branch,
		task.CommitID,
		task.CommitMessage,
		task.CurrentStage,
		task.Status,
		task.FailReason,
		task.ID,
	)
	body := dingtalkText(content)
	if strings.Contains(project.NotifyWebhook, "feishu") || strings.Contains(project.NotifyWebhook, "larksuite") {
		body = feishuText(content)
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, project.NotifyWebhook, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.Client.Do(req)
	if err != nil {
		n.Logger.Warn("notification failed", zap.Uint64("task_id", task.ID), zap.String("webhook", maskedURL(project.NotifyWebhook)), zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		err := fmt.Errorf("notification returned status %d", resp.StatusCode)
		n.Logger.Warn("notification failed", zap.Uint64("task_id", task.ID), zap.String("webhook", maskedURL(project.NotifyWebhook)), zap.Error(err))
		return err
	}
	return nil
}

func dingtalkText(content string) map[string]interface{} {
	return map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
}

func feishuText(content string) map[string]interface{} {
	return map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": content,
		},
	}
}

func maskedURL(raw string) string {
	if len(raw) <= 16 {
		return "******"
	}
	return raw[:12] + "******"
}
