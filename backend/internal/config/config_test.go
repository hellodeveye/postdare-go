package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithoutConfigUsesSafeDefaultsAndGeneratesSecrets(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("POSTDARE_GO_DATA_DIR", dataDir)
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	})

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected sqlite driver, got %q", cfg.Database.Driver)
	}
	if want := filepath.Join(dataDir, "postdare.db"); cfg.Database.Path != want {
		t.Fatalf("expected database path %q, got %q", want, cfg.Database.Path)
	}
	if cfg.JWT.Secret == "" || cfg.MCP.APIToken == "" {
		t.Fatal("expected generated jwt secret and mcp api token")
	}
	info, err := os.Stat(filepath.Join(dataDir, "secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected secrets mode 0600, got %o", got)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
data_dir: /should/not/win
database:
  driver: sqlite
jwt:
  secret: config-secret
mcp:
  api_token: config-token
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POSTDARE_GO_PORT", "9090")
	t.Setenv("POSTDARE_GO_DATA_DIR", t.TempDir())
	t.Setenv("POSTDARE_GO_DB_DRIVER", "mysql")
	t.Setenv("POSTDARE_GO_DB_DSN", "root:pass@tcp(127.0.0.1:3306)/postdare_go")
	t.Setenv("POSTDARE_GO_JWT_SECRET", "env-jwt")
	t.Setenv("POSTDARE_GO_MCP_API_TOKEN", "env-mcp")
	t.Setenv("POSTDARE_GO_ADMIN_PASSWORD", "env-admin-password")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port override, got %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "mysql" || cfg.Database.DSN == "" {
		t.Fatalf("expected mysql override, got %+v", cfg.Database)
	}
	if cfg.JWT.Secret != "env-jwt" || cfg.MCP.APIToken != "env-mcp" {
		t.Fatalf("expected env secrets to win, got jwt=%q mcp=%q", cfg.JWT.Secret, cfg.MCP.APIToken)
	}
	if cfg.AdminPassword != "env-admin-password" {
		t.Fatalf("expected admin password override, got %q", cfg.AdminPassword)
	}
}

func TestLoadMySQLRequiresDSN(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
data_dir: /tmp/postdare-test
database:
  driver: mysql
jwt:
  secret: jwt-secret
mcp:
  api_token: mcp-token
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("expected mysql without dsn to fail")
	}
}

func TestLoadPlaceholderSecretsUsePersistedSecrets(t *testing.T) {
	dataDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filepath.Join(dataDir, "secrets.yaml"), []byte(`
jwt:
  secret: persisted-jwt
mcp:
  api_token: persisted-mcp
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`
data_dir: `+dataDir+`
database:
  driver: sqlite
jwt:
  secret: please-change-this-secret
mcp:
  api_token: please-change-this-token
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWT.Secret != "persisted-jwt" || cfg.MCP.APIToken != "persisted-mcp" {
		t.Fatalf("expected persisted secrets, got jwt=%q mcp=%q", cfg.JWT.Secret, cfg.MCP.APIToken)
	}
}
