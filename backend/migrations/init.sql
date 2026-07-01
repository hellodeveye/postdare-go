CREATE DATABASE IF NOT EXISTS postdare_go DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE postdare_go;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(100) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(50) NOT NULL DEFAULT 'admin',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(100) NOT NULL,
  project_key VARCHAR(100) NOT NULL UNIQUE,
  git_provider VARCHAR(50) NOT NULL DEFAULT 'gitee',
  repo_url VARCHAR(500) NOT NULL,
  branch VARCHAR(100) NOT NULL DEFAULT 'main',
  repo_dir VARCHAR(500) NOT NULL,
  app_dir VARCHAR(500) NOT NULL,
  rollback_cmd TEXT,
  deploy_stages JSON,
  health_url VARCHAR(500),
  app_log_path VARCHAR(500),
  systemd_service VARCHAR(100),
  webhook_secret VARCHAR(255),
  auto_deploy_enabled TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS deploy_tasks (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  project_id BIGINT NOT NULL,
  trigger_type VARCHAR(50) NOT NULL,
  git_provider VARCHAR(50),
  branch VARCHAR(100),
  commit_id VARCHAR(100),
  commit_message TEXT,
  commit_author VARCHAR(100),
  status VARCHAR(50) NOT NULL,
  current_stage VARCHAR(100),
  fail_reason TEXT,
  log_file VARCHAR(500),
  started_at DATETIME,
  finished_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_project_id (project_id),
  INDEX idx_status (status),
  INDEX idx_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS deploy_task_stages (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  task_id BIGINT NOT NULL,
  name VARCHAR(100) NOT NULL,
  status VARCHAR(50) NOT NULL,
  started_at DATETIME,
  finished_at DATETIME,
  exit_code INT,
  error_message TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_task_id (task_id)
);

CREATE TABLE IF NOT EXISTS deploy_artifacts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  project_id BIGINT NOT NULL,
  task_id BIGINT NOT NULL,
  commit_id VARCHAR(100),
  artifact_path VARCHAR(500),
  backup_path VARCHAR(500),
  status VARCHAR(50),
  created_at DATETIME NOT NULL,
  INDEX idx_project_id (project_id),
  INDEX idx_task_id (task_id)
);

CREATE TABLE IF NOT EXISTS webhook_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  provider VARCHAR(50) NOT NULL,
  project_id BIGINT,
  project_key VARCHAR(100),
  event_type VARCHAR(100),
  branch VARCHAR(100),
  commit_id VARCHAR(100),
  commit_message TEXT,
  commit_author VARCHAR(100),
  delivery_id VARCHAR(255),
  signature_valid TINYINT(1) NOT NULL DEFAULT 0,
  handled TINYINT(1) NOT NULL DEFAULT 0,
  ignored_reason TEXT,
  raw_payload JSON,
  created_at DATETIME NOT NULL,
  INDEX idx_provider (provider),
  INDEX idx_project_id (project_id),
  INDEX idx_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS settings (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  `key` VARCHAR(120) NOT NULL UNIQUE,
  `value` TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

INSERT INTO users (username, password_hash, role, created_at, updated_at)
SELECT 'admin', '$2b$10$rMsUxGR5ZdawCno31eu0iukAEfSjh7qrmFbLDmFyM2dM/sSGsRtli', 'admin', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'admin');
