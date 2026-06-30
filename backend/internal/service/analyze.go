package service

import (
	"strings"
)

type FailureAnalysis struct {
	Summary          string   `json:"summary"`
	FailedStage      string   `json:"failed_stage"`
	PossibleCauses   []string `json:"possible_causes"`
	SuggestedActions []string `json:"suggested_actions"`
}

func AnalyzeFailureLog(failedStage string, logText string) FailureAnalysis {
	lower := strings.ToLower(logText)
	analysis := FailureAnalysis{
		Summary:        "未匹配到明确失败模式，请结合失败阶段和完整日志排查。",
		FailedStage:    failedStage,
		PossibleCauses: []string{},
		SuggestedActions: []string{
			"检查失败阶段前后的 stderr/stdout。",
			"在服务器上手动执行相同命令确认环境变量和权限。",
		},
	}
	add := func(summary, cause, action string) {
		analysis.Summary = summary
		analysis.PossibleCauses = append(analysis.PossibleCauses, cause)
		analysis.SuggestedActions = append(analysis.SuggestedActions, action)
	}
	switch {
	case strings.Contains(lower, "mvn test") && (strings.Contains(lower, "failure") || strings.Contains(lower, "failed")):
		add("单元测试阶段出现 Maven 测试失败。", "mvn test failed", "查看 surefire-reports，修复失败用例后重新部署。")
	case strings.Contains(lower, "connection refused"):
		add("命令或健康检查遇到连接拒绝。", "connection refused", "确认依赖服务已启动、端口正确、服务器防火墙放行。")
	case strings.Contains(lower, "permission denied"):
		add("部署命令遇到权限不足。", "permission denied", "检查脚本可执行权限、目录属主和 systemd 用户权限。")
	case strings.Contains(lower, "address already in use") || strings.Contains(lower, "port already in use"):
		add("应用启动失败，端口已被占用。", "port already in use", "定位占用端口的进程，调整服务端口或停止旧进程。")
	case strings.Contains(lower, "health check failed"):
		add("部署后健康检查失败。", "health check failed", "检查应用日志、健康检查 URL、启动耗时和依赖服务状态。")
	case strings.Contains(lower, "authentication failed") || strings.Contains(lower, "permission denied (publickey)"):
		add("Git 拉取阶段认证失败。", "git authentication failed", "检查部署服务器 SSH key、仓库权限和 known_hosts。")
	case strings.Contains(lower, "command timeout") || strings.Contains(lower, "deadline exceeded"):
		add("阶段命令执行超时。", "command timeout", "优化命令耗时，或在 config.yaml 中调大 deploy.command_timeout_minutes。")
	}
	if len(analysis.PossibleCauses) == 0 && failedStage != "" {
		analysis.Summary = "部署任务在 " + failedStage + " 阶段失败。"
	}
	return analysis
}
