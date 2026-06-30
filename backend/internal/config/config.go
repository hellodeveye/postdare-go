package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Deploy   DeployConfig   `yaml:"deploy"`
	AppLog   AppLogConfig   `yaml:"app_log"`
	MCP      MCPConfig      `yaml:"mcp"`
}

type ServerConfig struct {
	Port        int      `yaml:"port"`
	CORSOrigins []string `yaml:"cors_origins"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type DeployConfig struct {
	LogDir                string `yaml:"log_dir"`
	CommandTimeoutMinutes int    `yaml:"command_timeout_minutes"`
}

type AppLogConfig struct {
	MaxTailLines    int `yaml:"max_tail_lines"`
	MaxAllowedLines int `yaml:"max_allowed_lines"`
}

type MCPConfig struct {
	Enabled            bool   `yaml:"enabled"`
	AllowMutationTools bool   `yaml:"allow_mutation_tools"`
	APIToken           string `yaml:"api_token"`
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = "config.yaml"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8088
	}
	if c.JWT.ExpireHours == 0 {
		c.JWT.ExpireHours = 72
	}
	if c.Deploy.LogDir == "" {
		c.Deploy.LogDir = "/data/postdare-go/logs/deploy"
	}
	if c.Deploy.CommandTimeoutMinutes == 0 {
		c.Deploy.CommandTimeoutMinutes = 30
	}
	if c.AppLog.MaxTailLines == 0 {
		c.AppLog.MaxTailLines = 500
	}
	if c.AppLog.MaxAllowedLines == 0 {
		c.AppLog.MaxAllowedLines = 5000
	}
}

func (c *Config) JWTDuration() time.Duration {
	return time.Duration(c.JWT.ExpireHours) * time.Hour
}

func (c *Config) CommandTimeout() time.Duration {
	return time.Duration(c.Deploy.CommandTimeoutMinutes) * time.Minute
}
