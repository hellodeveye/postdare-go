package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"

	"postdare-go/backend/internal/model"
)

const (
	TemplateDingTalkText = "dingtalk_text"
	TemplateWeComText    = "wecom_text"
	TemplateFeishuText   = "feishu_text"
	TemplateGenericJSON  = "generic_json"
)

const defaultMessageTemplate = `Postdare Go {{ .Scene }}
项目: {{ .Project.Name }}
Git: {{ .Task.GitProvider }}
触发: {{ .Task.TriggerType }}
分支: {{ .Task.Branch }}
commit: {{ .Task.CommitID }}
消息: {{ .Task.CommitMessage }}
阶段: {{ .Task.CurrentStage }}
状态: {{ .Task.Status }}
失败原因: {{ .Task.FailReason }}
任务ID: {{ .Task.ID }}
耗时: {{ .Duration }}`

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

type MessageContext struct {
	Project  model.Project
	Task     model.DeployTask
	Scene    string
	Duration string
}

func (n *Notifier) SendOutboundWebhook(project model.Project, task model.DeployTask, cfg model.OutboundWebhookStageConfig) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil
	}
	content, err := renderMessage(project, task, cfg.MessageTemplate)
	if err != nil {
		return err
	}
	raw, err := renderPayload(cfg, content)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.Client.Do(req)
	if err != nil {
		n.Logger.Warn("outbound webhook failed", zap.Uint64("task_id", task.ID), zap.String("webhook", maskedURL(cfg.URL)), zap.Error(err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		err := fmt.Errorf("outbound webhook returned status %d", resp.StatusCode)
		n.Logger.Warn("outbound webhook failed", zap.Uint64("task_id", task.ID), zap.String("webhook", maskedURL(cfg.URL)), zap.Error(err))
		return err
	}
	return nil
}

func renderMessage(project model.Project, task model.DeployTask, messageTemplate string) (string, error) {
	if strings.TrimSpace(messageTemplate) == "" {
		messageTemplate = defaultMessageTemplate
	}
	tpl, err := template.New("outbound_webhook_message").Parse(messageTemplate)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, MessageContext{
		Project:  project,
		Task:     task,
		Scene:    taskScene(task),
		Duration: taskDuration(task),
	}); err != nil {
		return "", err
	}
	return out.String(), nil
}

func renderPayload(cfg model.OutboundWebhookStageConfig, content string) ([]byte, error) {
	templateName := strings.TrimSpace(cfg.Template)
	if templateName == "" {
		templateName = TemplateDingTalkText
	}
	if templateName == TemplateGenericJSON {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(content), &raw); err != nil {
			return nil, fmt.Errorf("generic_json message_template must render valid JSON: %w", err)
		}
		return raw, nil
	}
	var body map[string]interface{}
	switch templateName {
	case TemplateFeishuText:
		body = feishuText(content)
	case TemplateDingTalkText, TemplateWeComText:
		body = dingtalkText(content)
	default:
		return nil, fmt.Errorf("unsupported outbound webhook template %q", templateName)
	}
	return json.Marshal(body)
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

func taskScene(task model.DeployTask) string {
	switch task.Status {
	case model.TaskSuccess:
		return "部署成功"
	case model.TaskFailed:
		return "部署失败"
	case model.TaskRollbacked:
		return "回滚成功"
	case model.TaskCanceled:
		return "部署取消"
	default:
		return "部署进行中"
	}
}

func taskDuration(task model.DeployTask) string {
	if task.StartedAt == nil {
		return ""
	}
	end := time.Now()
	if task.FinishedAt != nil {
		end = *task.FinishedAt
	}
	return end.Sub(*task.StartedAt).Round(time.Second).String()
}

func maskedURL(raw string) string {
	if len(raw) <= 16 {
		return "******"
	}
	return raw[:12] + "******"
}
