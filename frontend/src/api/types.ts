export type GitProvider = "gitee" | "github";
export type DeployStatus = "pending" | "running" | "success" | "failed" | "canceled" | "rollbacked";
export type ProjectStageType = "command" | "health_check" | "outbound_webhook";
export type ProjectStageRunWhen = "success" | "failed" | "always";
export type OutboundWebhookTemplate = "dingtalk_text" | "wecom_text" | "feishu_text" | "generic_json";

export interface ProjectStageBase {
  name: string;
  type: ProjectStageType;
  enabled: boolean;
  run_when?: ProjectStageRunWhen;
  continue_on_error?: boolean;
}

export interface CommandProjectStage extends ProjectStageBase {
  type: "command";
  config: {
    command: string;
  };
}

export interface HealthCheckProjectStage extends ProjectStageBase {
  type: "health_check";
  config?: {
    url?: string;
  };
}

export interface OutboundWebhookProjectStage extends ProjectStageBase {
  type: "outbound_webhook";
  config?: {
    url?: string;
    template?: OutboundWebhookTemplate;
    message_template?: string;
  };
}

export type ProjectStage = CommandProjectStage | HealthCheckProjectStage | OutboundWebhookProjectStage;

export interface Project {
  id: number;
  name: string;
  project_key: string;
  git_provider: GitProvider;
  branch: string;
  app_dir: string;
  rollback_cmd?: string;
  deploy_stages?: ProjectStage[];
  app_log_path?: string;
  webhook_secret?: string;
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
