package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
pki:
  bundle_dir: /etc/ploy/pki
runtime:
  plugins:
    - name: local
      module: internal
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if expect := ":8443"; cfg.HTTP.Listen != expect {
		t.Fatalf("HTTP.Listen = %q, want %q", cfg.HTTP.Listen, expect)
	}
	if expect := ":9100"; cfg.Metrics.Listen != expect {
		t.Fatalf("Metrics.Listen = %q, want %q", cfg.Metrics.Listen, expect)
	}
	if cfg.Admin.Socket == "" {
		t.Fatal("Admin.Socket should default to non-empty path")
	}
	if cfg.ControlPlane.HeartbeatInterval != 10*time.Second {
		t.Fatalf("HeartbeatInterval = %v, want 10s", cfg.ControlPlane.HeartbeatInterval)
	}
	if cfg.ControlPlane.AssignmentPollInterval != 5*time.Second {
		t.Fatalf("AssignmentPollInterval = %v, want 5s", cfg.ControlPlane.AssignmentPollInterval)
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
	if cfg.Scheduler.PrepInterval != 0 {
		t.Fatalf("PrepInterval = %v, want 0", cfg.Scheduler.PrepInterval)
	}
	if cfg.Scheduler.PrepMaxAttempts != 3 {
		t.Fatalf("PrepMaxAttempts = %d, want 3", cfg.Scheduler.PrepMaxAttempts)
	}
	if cfg.Scheduler.PrepRetryDelay != 30*time.Second {
		t.Fatalf("PrepRetryDelay = %v, want 30s", cfg.Scheduler.PrepRetryDelay)
	}
	if cfg.PKI.RenewBefore != time.Hour {
		t.Fatalf("PKI.RenewBefore = %v, want 1h", cfg.PKI.RenewBefore)
	}
	if cfg.Runtime.DefaultAdapter != "local" {
		t.Fatalf("Runtime.DefaultAdapter = %q, want local", cfg.Runtime.DefaultAdapter)
	}
	if len(cfg.Runtime.Plugins) != 1 {
		t.Fatalf("Runtime.Plugins length = %d, want 1", len(cfg.Runtime.Plugins))
	}
	if cfg.Runtime.Plugins[0].Name != "local" {
		t.Fatalf("Runtime.Plugins[0].Name = %q, want local", cfg.Runtime.Plugins[0].Name)
	}
}

func TestLoadConfigCustomizations(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
http:
  listen: 127.0.0.1:18443
metrics:
  listen: 127.0.0.1:19100
admin:
  socket: /run/custom-ployd.sock
control_plane:
  endpoint: https://beacon.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/beacon.pem
  key: /etc/ploy/pki/beacon-key.pem
  heartbeat_interval: 2s
  assignment_poll_interval: 3s
pki:
  bundle_dir: /var/lib/ploy/pki
  renew_before: 12m
runtime:
  default_adapter: k8s
  plugins:
    - name: k8s
      module: github.com/example/ployd-k8s
      config:
        address: https://k8s.example.com
scheduler:
  stale_job_recovery_interval: 0s
  node_stale_after: 2m
  prep_interval: 5s
  prep_max_attempts: 4
  prep_retry_delay: 45s
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Listen != "127.0.0.1:18443" {
		t.Fatalf("HTTP.Listen = %q", cfg.HTTP.Listen)
	}
	if cfg.Metrics.Listen != "127.0.0.1:19100" {
		t.Fatalf("Metrics.Listen = %q", cfg.Metrics.Listen)
	}
	if cfg.Admin.Socket != "/run/custom-ployd.sock" {
		t.Fatalf("Admin.Socket = %q", cfg.Admin.Socket)
	}
	if cfg.ControlPlane.HeartbeatInterval != 2*time.Second {
		t.Fatalf("HeartbeatInterval = %v", cfg.ControlPlane.HeartbeatInterval)
	}
	if cfg.ControlPlane.AssignmentPollInterval != 3*time.Second {
		t.Fatalf("AssignmentPollInterval = %v", cfg.ControlPlane.AssignmentPollInterval)
	}
	if cfg.PKI.BundleDir != "/var/lib/ploy/pki" {
		t.Fatalf("PKI.BundleDir = %q", cfg.PKI.BundleDir)
	}
	if cfg.PKI.RenewBefore != 12*time.Minute {
		t.Fatalf("PKI.RenewBefore = %v", cfg.PKI.RenewBefore)
	}
	if cfg.Runtime.DefaultAdapter != "k8s" {
		t.Fatalf("Runtime.DefaultAdapter = %q", cfg.Runtime.DefaultAdapter)
	}
	if len(cfg.Runtime.Plugins) != 1 {
		t.Fatalf("Runtime.Plugins length = %d", len(cfg.Runtime.Plugins))
	}
	if cfg.Runtime.Plugins[0].Module != "github.com/example/ployd-k8s" {
		t.Fatalf("Runtime.Plugins[0].Module = %q", cfg.Runtime.Plugins[0].Module)
	}
	if cfg.Runtime.Plugins[0].Config["address"] != "https://k8s.example.com" {
		t.Fatalf("Runtime.Plugins[0].Config[address] = %v", cfg.Runtime.Plugins[0].Config["address"])
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
	if cfg.Scheduler.PrepInterval != 5*time.Second {
		t.Fatalf("PrepInterval = %v, want 5s", cfg.Scheduler.PrepInterval)
	}
	if cfg.Scheduler.PrepMaxAttempts != 4 {
		t.Fatalf("PrepMaxAttempts = %d, want 4", cfg.Scheduler.PrepMaxAttempts)
	}
	if cfg.Scheduler.PrepRetryDelay != 45*time.Second {
		t.Fatalf("PrepRetryDelay = %v, want 45s", cfg.Scheduler.PrepRetryDelay)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
http:
  listen: :8443
control_plane:
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
}

func TestLoadConfigGitLab(t *testing.T) {
	t.Helper()

	tests := []struct {
		name       string
		yaml       string
		wantDomain string
		wantToken  string
	}{
		{
			name: "gitlab_with_domain_and_token",
			yaml: `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
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
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
`,
			wantDomain: "https://gitlab.example.com",
			wantToken:  "",
		},
		{
			name: "gitlab_empty",
			yaml: `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
pki:
  bundle_dir: /etc/ploy/pki
`,
			wantDomain: "",
			wantToken:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ployd.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := config.Load(path)
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
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
  extra: should_fail
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() succeeded, want error for unknown gitlab.extra field")
	}
}

func TestLoadConfigGitLabTokenFile(t *testing.T) {
	t.Helper()

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
			tokenFile: "", // will be set to absolute path
			tokenPerm: 0600,
			tokenData: "glpat-from-file-123",
			wantToken: "glpat-from-file-123",
			wantErr:   false,
		},
		{
			name:      "token_file_relative_path",
			tokenFile: "gitlab-token.txt",
			tokenPerm: 0600,
			tokenData: "glpat-relative-path",
			wantToken: "glpat-relative-path",
			wantErr:   false,
		},
		{
			name:        "token_file_insecure_permissions",
			tokenFile:   "",
			tokenPerm:   0644,
			tokenData:   "glpat-insecure",
			wantErr:     true,
			errContains: "insecure permissions",
		},
		{
			name:        "token_file_empty",
			tokenFile:   "",
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

			// Create token file
			actualTokenPath := filepath.Join(dir, filepath.Base(tokenFilePath))
			if err := os.WriteFile(actualTokenPath, []byte(tt.tokenData), tt.tokenPerm); err != nil {
				t.Fatalf("write token file: %v", err)
			}

			// Create config with token_file
			configYAML := `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
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
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
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
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "ployd.yaml")
	tokenPath := filepath.Join(dir, "token")

	// Create token file
	if err := os.WriteFile(tokenPath, []byte("glpat-from-file"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	// Config with both token and token_file: token should take precedence
	configYAML := `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
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

	// Inline token should take precedence over token_file
	if cfg.GitLab.Token != "glpat-inline" {
		t.Errorf("GitLab.Token = %q, want %q (inline token should take precedence)", cfg.GitLab.Token, "glpat-inline")
	}
}

// Absolute path resolution for gitlab.token_file should be accepted as-is.
func TestLoadConfigGitLabTokenFile_AbsolutePath(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "ployd.yaml")
	tokenPath := filepath.Join(dir, "token-abs")

	if err := os.WriteFile(tokenPath, []byte("glpat-abs-path"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	configYAML := "\ncontrol_plane:\n  endpoint: https://control.example.com\n  ca: /etc/ploy/pki/ca.pem\n  certificate: /etc/ploy/pki/node.pem\n  key: /etc/ploy/pki/node-key.pem\npki:\n  bundle_dir: /etc/ploy/pki\ngitlab:\n  domain: https://gitlab.example.com\n  token_file: " + tokenPath + "\n"
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

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
