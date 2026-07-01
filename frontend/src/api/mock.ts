import type { DashboardSummary, DeployTask, Project, WebhookEvent } from "./types";

export const mockProjects: Project[] = [
  {
    id: 1,
    name: "my-app",
    project_key: "my-app",
    git_provider: "github",
    repo_url: "git@github.com:acme/my-app.git",
    branch: "main",
    repo_dir: "/data/repos/my-app",
    app_dir: "/data/apps/my-app",
    deploy_stages: [
      { name: "pull_code", type: "command", config: { command: "cd /data/repos/my-app && git fetch --all && git reset --hard origin/main" }, enabled: true },
      { name: "unit_test", type: "command", config: { command: "cd /data/repos/my-app && mvn clean test" }, enabled: true },
      { name: "integration_test", type: "command", config: { command: "cd /data/repos/my-app && mvn verify" }, enabled: true },
      { name: "build", type: "command", config: { command: "cd /data/repos/my-app && mvn package -DskipTests" }, enabled: true },
      { name: "deploy", type: "command", config: { command: "bash /data/apps/my-app/deploy.sh" }, enabled: true },
      { name: "health_check", type: "health_check", config: { url: "http://127.0.0.1:8080/actuator/health" }, enabled: true },
      {
        name: "outbound_webhook",
        type: "outbound_webhook",
        run_when: "always",
        continue_on_error: true,
        config: { url: "https://hooks.example.com/notify", template: "feishu_text" },
        enabled: true
      }
    ],
    rollback_cmd: "bash /data/apps/my-app/rollback.sh",
    app_log_path: "/data/apps/my-app/logs/app.log",
    systemd_service: "my-app",
    webhook_secret: "******",
    auto_deploy_enabled: true,
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date().toISOString()
  },
  {
    id: 2,
    name: "ship-worker",
    project_key: "ship-worker",
    git_provider: "gitee",
    repo_url: "git@gitee.com:acme/ship-worker.git",
    branch: "develop",
    repo_dir: "/data/repos/ship-worker",
    app_dir: "/data/apps/ship-worker",
    deploy_stages: [
      { name: "deploy", type: "command", config: { command: "bash /data/apps/ship-worker/deploy.sh" }, enabled: true }
    ],
    rollback_cmd: "bash /data/apps/ship-worker/rollback.sh",
    app_log_path: "/data/apps/ship-worker/logs/app.log",
    auto_deploy_enabled: false,
    created_at: new Date(Date.now() - 172800000).toISOString(),
    updated_at: new Date(Date.now() - 3600000).toISOString()
  }
];

export const mockTasks: DeployTask[] = [
  {
    id: 104,
    project_id: 1,
    project: mockProjects[0],
    trigger_type: "webhook",
    git_provider: "github",
    branch: "main",
    commit_id: "8f3a9c2e4b7a",
    commit_message: "fix health check timeout",
    commit_author: "kim",
    status: "success",
    current_stage: "outbound_webhook",
    started_at: new Date(Date.now() - 1800000).toISOString(),
    finished_at: new Date(Date.now() - 1500000).toISOString(),
    created_at: new Date(Date.now() - 1900000).toISOString(),
    stages: [
      { id: 1, task_id: 104, name: "pull_code", status: "success" },
      { id: 2, task_id: 104, name: "unit_test", status: "success" },
      { id: 3, task_id: 104, name: "integration_test", status: "success" },
      { id: 4, task_id: 104, name: "build", status: "success" },
      { id: 5, task_id: 104, name: "deploy", status: "success" },
      { id: 6, task_id: 104, name: "health_check", status: "success" }
    ]
  },
  {
    id: 103,
    project_id: 2,
    project: mockProjects[1],
    trigger_type: "manual",
    git_provider: "gitee",
    branch: "develop",
    commit_id: "c31d42a0",
    commit_message: "add import job",
    commit_author: "lin",
    status: "failed",
    current_stage: "health_check",
    fail_reason: "health check failed: connection refused",
    started_at: new Date(Date.now() - 5400000).toISOString(),
    finished_at: new Date(Date.now() - 5000000).toISOString(),
    created_at: new Date(Date.now() - 5500000).toISOString(),
    stages: [
      { id: 7, task_id: 103, name: "pull_code", status: "success" },
      { id: 8, task_id: 103, name: "unit_test", status: "skipped" },
      { id: 9, task_id: 103, name: "build", status: "success" },
      { id: 10, task_id: 103, name: "deploy", status: "success" },
      { id: 11, task_id: 103, name: "health_check", status: "failed", error_message: "connection refused" }
    ]
  }
];

export const mockWebhookEvents: WebhookEvent[] = [
  {
    id: 41,
    provider: "github",
    project_id: 1,
    project_key: "my-app",
    event_type: "push",
    branch: "main",
    commit_id: "8f3a9c2e4b7a",
    commit_message: "fix health check timeout",
    commit_author: "kim",
    delivery_id: "8e8d8b7a",
    signature_valid: true,
    handled: true,
    created_at: new Date(Date.now() - 1900000).toISOString()
  },
  {
    id: 40,
    provider: "gitee",
    project_id: 2,
    project_key: "ship-worker",
    event_type: "push",
    branch: "feature/import",
    commit_id: "2b7ca11",
    signature_valid: true,
    handled: false,
    ignored_reason: "branch mismatch",
    created_at: new Date(Date.now() - 7300000).toISOString()
  }
];

export function mockResponse(path: string, init?: RequestInit): unknown {
  const url = new URL(path, "http://mock.local");
  const pathname = url.pathname;
  if (pathname.endsWith("/auth/login")) {
    return { data: { token: "mock-token", user: { id: 1, username: "admin", role: "admin" } } };
  }
  if (pathname.endsWith("/auth/me")) {
    return { data: { id: 1, username: "admin", role: "admin", actor: "mock" } };
  }
  if (pathname.endsWith("/dashboard/summary")) {
    const summary: DashboardSummary = {
      project_total: mockProjects.length,
      today_deploy_total: 8,
      today_success_total: 6,
      today_failed_total: 2,
      success_rate: 0.75,
      recent_failed_tasks: mockTasks.filter((task) => task.status === "failed")
    };
    return { data: summary };
  }
  if (pathname.endsWith("/dashboard/recent-deploy-tasks")) {
    return { data: mockTasks };
  }
  if (pathname.endsWith("/projects") && init?.method === "POST") {
    return { data: { ...JSON.parse(String(init.body ?? "{}")), id: 99, created_at: new Date().toISOString() } };
  }
  if (pathname.endsWith("/projects")) {
    return { data: mockProjects, pagination: { page: 1, page_size: 20, total: mockProjects.length } };
  }
  const projectMatch = pathname.match(/\/projects\/(\d+)$/);
  if (projectMatch) {
    return { data: mockProjects.find((project) => project.id === Number(projectMatch[1])) ?? mockProjects[0] };
  }
  if (pathname.includes("/app-logs")) {
    return { data: { log: "[app] server started\n[app] health check ok\n[app] request completed in 42ms\n" } };
  }
  if (pathname.endsWith("/deploy-tasks") && init?.method === "POST") {
    return { data: { ...mockTasks[0], id: Math.floor(Math.random() * 10000), status: "pending", trigger_type: "manual" } };
  }
  if (pathname.endsWith("/rollback-tasks") && init?.method === "POST") {
    return { data: { ...mockTasks[0], id: Math.floor(Math.random() * 10000), status: "pending", trigger_type: "rollback" } };
  }
  if (pathname.endsWith("/deploy-tasks")) {
    return { data: mockTasks, pagination: { page: 1, page_size: 20, total: mockTasks.length } };
  }
  const taskMatch = pathname.match(/\/deploy-tasks\/(\d+)$/);
  if (taskMatch) {
    return { data: mockTasks.find((task) => task.id === Number(taskMatch[1])) ?? mockTasks[0] };
  }
  if (pathname.endsWith("/stages")) {
    return { data: mockTasks[0].stages ?? [] };
  }
  if (pathname.endsWith("/logs")) {
    return { data: { log: "[pull_code] git fetch --all\n[build] mvn package -DskipTests\n[health_check] health check passed\n" } };
  }
  if (pathname.endsWith("/analysis")) {
    return { data: { summary: "健康检查失败。", failed_stage: "health_check", possible_causes: ["connection refused"], suggested_actions: ["检查应用进程和端口。"] } };
  }
  if (pathname.endsWith("/webhook-events")) {
    return { data: mockWebhookEvents, pagination: { page: 1, page_size: 20, total: mockWebhookEvents.length } };
  }
  if (pathname.endsWith("/settings")) {
    return { data: { mcp: { enabled: true, allow_mutation_tools: false, api_token: "ple******ken" }, deploy: { log_dir: "/data/postdare-go/logs/deploy" } } };
  }
  return { data: null };
}
