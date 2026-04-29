package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func writeConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigDefaultsAndCustomizations(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		checks func(t *testing.T, cfg config.Config)
	}{
		{
			name: "defaults",
			yaml: "\npki:\n  bundle_dir: /etc/ploy/pki\n",
			checks: func(t *testing.T, cfg config.Config) {
				if cfg.HTTP.Listen != ":8080" {
					t.Fatalf("HTTP.Listen = %q, want :8080", cfg.HTTP.Listen)
				}
				if cfg.Metrics.Listen != ":9100" {
					t.Fatalf("Metrics.Listen = %q, want :9100", cfg.Metrics.Listen)
				}
				if cfg.Admin.Socket == "" {
					t.Fatal("Admin.Socket should default to non-empty path")
				}
				if cfg.Scheduler.StaleJobRecoveryInterval != 30*time.Second {
					t.Fatalf("StaleJobRecoveryInterval = %v, want 30s", cfg.Scheduler.StaleJobRecoveryInterval)
				}
				if cfg.Scheduler.BatchSchedulerInterval != 5*time.Second {
					t.Fatalf("BatchSchedulerInterval = %v, want 5s", cfg.Scheduler.BatchSchedulerInterval)
				}
				if cfg.Scheduler.NodeStaleAfter != time.Minute {
					t.Fatalf("NodeStaleAfter = %v, want 1m", cfg.Scheduler.NodeStaleAfter)
				}
				if cfg.PKI.RenewBefore != time.Hour {
					t.Fatalf("PKI.RenewBefore = %v, want 1h", cfg.PKI.RenewBefore)
				}
			},
		},
		{
			name: "custom values",
			yaml: `
http:
  listen: 127.0.0.1:18443
metrics:
  listen: 127.0.0.1:19100
admin:
  socket: /run/custom-ployd.sock
pki:
  bundle_dir: /var/lib/ploy/pki
  renew_before: 12m
scheduler:
  stale_job_recovery_interval: 0s
  node_stale_after: 2m
`,
			checks: func(t *testing.T, cfg config.Config) {
				if cfg.HTTP.Listen != "127.0.0.1:18443" {
					t.Fatalf("HTTP.Listen = %q", cfg.HTTP.Listen)
				}
				if cfg.Metrics.Listen != "127.0.0.1:19100" {
					t.Fatalf("Metrics.Listen = %q", cfg.Metrics.Listen)
				}
				if cfg.Admin.Socket != "/run/custom-ployd.sock" {
					t.Fatalf("Admin.Socket = %q", cfg.Admin.Socket)
				}
				if cfg.PKI.BundleDir != "/var/lib/ploy/pki" {
					t.Fatalf("PKI.BundleDir = %q", cfg.PKI.BundleDir)
				}
				if cfg.PKI.RenewBefore != 12*time.Minute {
					t.Fatalf("PKI.RenewBefore = %v", cfg.PKI.RenewBefore)
				}
				if cfg.Scheduler.StaleJobRecoveryInterval != 0 {
					t.Fatalf("StaleJobRecoveryInterval = %v, want 0", cfg.Scheduler.StaleJobRecoveryInterval)
				}
				if cfg.Scheduler.BatchSchedulerInterval != 5*time.Second {
					t.Fatalf("BatchSchedulerInterval = %v, want 5s", cfg.Scheduler.BatchSchedulerInterval)
				}
				if cfg.Scheduler.NodeStaleAfter != 2*time.Minute {
					t.Fatalf("NodeStaleAfter = %v, want 2m", cfg.Scheduler.NodeStaleAfter)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.yaml))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			tt.checks(t, cfg)
		})
	}
}

func TestLoadConfigValidation(t *testing.T) {
	path := writeConfig(t, "\nhttp:\n  listen: :8443\n")
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
}

func TestLoadConfigGitLab(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantDomain string
		wantToken  string
	}{
		{
			name: "gitlab_with_domain_and_token",
			yaml: `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
  token: glpat-test-token-123
`,
			wantDomain: "https://gitlab.example.com",
			wantToken:  "glpat-test-token-123",
		},
		{
			name: "gitlab_with_domain_only",
			yaml: `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
`,
			wantDomain: "https://gitlab.example.com",
		},
		{
			name:       "gitlab_empty",
			yaml:       "\npki:\n  bundle_dir: /etc/ploy/pki\n",
			wantDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.yaml))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.GitLab.Domain != tt.wantDomain {
				t.Errorf("GitLab.Domain = %q, want %q", cfg.GitLab.Domain, tt.wantDomain)
			}
			if cfg.GitLab.Token != tt.wantToken {
				t.Errorf("GitLab.Token = %q, want %q", cfg.GitLab.Token, tt.wantToken)
			}
		})
	}
}

func TestLoadConfigGitLabUnknownFieldFails(t *testing.T) {
	raw := `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
  extra: should_fail
`
	if _, err := config.Load(writeConfig(t, raw)); err == nil {
		t.Fatal("Load() succeeded, want error for unknown gitlab.extra field")
	}
}

func TestLoadConfig_NodeSectionsRejectedForServer(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"control_plane_section", "\ncontrol_plane:\n  endpoint: https://control.example.com\n"},
		{"worker_section", "\nworker:\n  task_concurrency: 8\n"},
		{"runtime_section", "\nruntime:\n  default_adapter: local\n"},
		{"transfers_section", "\ntransfers:\n  base_dir: /tmp/transfers\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := config.Load(writeConfig(t, tt.raw)); err == nil {
				t.Fatalf("Load() succeeded, want error for node section %q", tt.name)
			}
		})
	}
}

func TestLoadConfigGitLabTokenFile(t *testing.T) {
	tests := []struct {
		name        string
		tokenFile   string
		tokenPerm   os.FileMode
		tokenData   string
		wantToken   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "token_file_absolute_path",
			tokenPerm: 0600,
			tokenData: "glpat-from-file-123",
			wantToken: "glpat-from-file-123",
		},
		{
			name:      "token_file_relative_path",
			tokenFile: "gitlab-token.txt",
			tokenPerm: 0600,
			tokenData: "glpat-relative-path",
			wantToken: "glpat-relative-path",
		},
		{
			name:        "token_file_insecure_permissions",
			tokenPerm:   0644,
			tokenData:   "glpat-insecure",
			wantErr:     true,
			errContains: "insecure permissions",
		},
		{
			name:        "token_file_empty",
			tokenPerm:   0600,
			tokenData:   "",
			wantErr:     true,
			errContains: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "ployd.yaml")

			var tokenFilePath string
			if tt.tokenFile == "" {
				tokenFilePath = filepath.Join(dir, "token")
			} else {
				tokenFilePath = tt.tokenFile
			}

			actualTokenPath := filepath.Join(dir, filepath.Base(tokenFilePath))
			if err := os.WriteFile(actualTokenPath, []byte(tt.tokenData), tt.tokenPerm); err != nil {
				t.Fatalf("write token file: %v", err)
			}

			configYAML := `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
  token_file: ` + filepath.Base(tokenFilePath)
			if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := config.Load(configPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Load() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.GitLab.Token != tt.wantToken {
				t.Errorf("GitLab.Token = %q, want %q", cfg.GitLab.Token, tt.wantToken)
			}
		})
	}
}

func TestLoadConfigGitLabTokenPrecedence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ployd.yaml")
	tokenPath := filepath.Join(dir, "token")

	if err := os.WriteFile(tokenPath, []byte("glpat-from-file"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	configYAML := `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
  token: glpat-inline
  token_file: token
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitLab.Token != "glpat-inline" {
		t.Errorf("GitLab.Token = %q, want %q (inline token should take precedence)", cfg.GitLab.Token, "glpat-inline")
	}
}

func TestLoadConfigGitLabTokenFile_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ployd.yaml")
	tokenPath := filepath.Join(dir, "token-abs")

	if err := os.WriteFile(tokenPath, []byte("glpat-abs-path"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	configYAML := "\npki:\n  bundle_dir: /etc/ploy/pki\ngitlab:\n  domain: https://gitlab.example.com\n  token_file: " + tokenPath + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.GitLab.Token, "glpat-abs-path"; got != want {
		t.Errorf("GitLab.Token = %q, want %q (absolute path)", got, want)
	}
}
