export type GitProvider = "gitee" | "github";
export type DeployStatus = "pending" | "running" | "success" | "failed" | "canceled" | "rollbacked";

export interface Project {
  id: number;
  name: string;
  project_key: string;
  git_provider: GitProvider;
  repo_url: string;
  branch: string;
  repo_dir: string;
  app_dir: string;
  pull_cmd?: string;
  unit_test_cmd?: string;
  integration_test_cmd?: string;
  build_cmd?: string;
  deploy_cmd?: string;
  rollback_cmd?: string;
  health_url?: string;
  app_log_path?: string;
  systemd_service?: string;
  webhook_secret?: string;
  notify_webhook?: string;
  auto_deploy_enabled: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface DeployTask {
  id: number;
  project_id: number;
  project?: Project;
  trigger_type: string;
  git_provider?: GitProvider;
  branch?: string;
  commit_id?: string;
  commit_message?: string;
  commit_author?: string;
  status: DeployStatus;
  current_stage?: string;
  fail_reason?: string;
  log_file?: string;
  started_at?: string | null;
  finished_at?: string | null;
  created_at?: string;
  updated_at?: string;
  stages?: DeployTaskStage[];
}

export interface DeployTaskStage {
  id: number;
  task_id: number;
  name: string;
  status: string;
  started_at?: string | null;
  finished_at?: string | null;
  exit_code?: number | null;
  error_message?: string;
}

export interface WebhookEvent {
  id: number;
  provider: GitProvider;
  project_id?: number;
  project_key?: string;
  event_type?: string;
  branch?: string;
  commit_id?: string;
  commit_message?: string;
  commit_author?: string;
  delivery_id?: string;
  signature_valid: boolean;
  handled: boolean;
  ignored_reason?: string;
  created_at?: string;
}

export interface DashboardSummary {
  project_total: number;
  today_deploy_total: number;
  today_success_total: number;
  today_failed_total: number;
  success_rate: number;
  recent_failed_tasks: DeployTask[];
}

export interface ListResponse<T> {
  data: T[];
  pagination: {
    page: number;
    page_size: number;
    total: number;
  };
  request_id?: string;
}

export interface DataResponse<T> {
  data: T;
  request_id?: string;
}
