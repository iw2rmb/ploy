package config_test

import (
	"os"
	"path/filepath"
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
				if cfg.Scheduler.WaveSchedulerInterval != 5*time.Second {
					t.Fatalf("WaveSchedulerInterval = %v, want 5s", cfg.Scheduler.WaveSchedulerInterval)
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
				if cfg.Scheduler.WaveSchedulerInterval != 5*time.Second {
					t.Fatalf("WaveSchedulerInterval = %v, want 5s", cfg.Scheduler.WaveSchedulerInterval)
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

func TestLoadConfigGitLabYAMLRejected(t *testing.T) {
	raw := `
pki:
  bundle_dir: /etc/ploy/pki
gitlab:
  domain: https://gitlab.example.com
`
	if _, err := config.Load(writeConfig(t, raw)); err == nil {
		t.Fatal("Load() succeeded, want error for YAML GitLab configuration")
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
