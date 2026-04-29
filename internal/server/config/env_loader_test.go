package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestLoadFromEnv_Defaults(t *testing.T) {
	clearEnvForLoadFromEnv(t)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.HTTP.Listen != ":8080" {
		t.Fatalf("HTTP.Listen = %q, want :8080", cfg.HTTP.Listen)
	}
	if cfg.Metrics.Listen != ":9100" {
		t.Fatalf("Metrics.Listen = %q, want :9100", cfg.Metrics.Listen)
	}
	if cfg.Admin.Socket != "/run/ployd.sock" {
		t.Fatalf("Admin.Socket = %q, want /run/ployd.sock", cfg.Admin.Socket)
	}
	if !cfg.Auth.BearerTokens.Enabled {
		t.Fatal("Auth.BearerTokens.Enabled = false, want true")
	}
}

func TestLoadFromEnv_Overrides(t *testing.T) {
	clearEnvForLoadFromEnv(t)
	t.Setenv("PLOYD_HTTP_LISTEN", "127.0.0.1:18080")
	t.Setenv("PLOYD_METRICS_LISTEN", "127.0.0.1:19100")
	t.Setenv("PLOYD_HTTP_READ_TIMEOUT", "21s")
	t.Setenv("PLOYD_AUTH_BEARER_TOKENS_ENABLED", "false")
	t.Setenv("PLOYD_LOG_JSON", "true")
	t.Setenv("PLOYD_LOG_MAX_SIZE_MB", "128")
	t.Setenv("PLOYD_LOG_STATIC_FIELDS", `{"cluster":"test","role":"server"}`)
	t.Setenv("PLOYD_SCHEDULER_BATCH_SCHEDULER_INTERVAL", "0s")
	t.Setenv("PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL", "45s")
	t.Setenv("PLOY_GITLAB_DOMAIN", "https://gitlab.example.com")
	t.Setenv("PLOY_GITLAB_TOKEN", "glpat-test")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.HTTP.Listen != "127.0.0.1:18080" {
		t.Fatalf("HTTP.Listen = %q", cfg.HTTP.Listen)
	}
	if cfg.Metrics.Listen != "127.0.0.1:19100" {
		t.Fatalf("Metrics.Listen = %q", cfg.Metrics.Listen)
	}
	if cfg.HTTP.ReadTimeout != 21*time.Second {
		t.Fatalf("HTTP.ReadTimeout = %v, want 21s", cfg.HTTP.ReadTimeout)
	}
	if cfg.Auth.BearerTokens.Enabled {
		t.Fatal("Auth.BearerTokens.Enabled = true, want false")
	}
	if !cfg.Logging.JSON {
		t.Fatal("Logging.JSON = false, want true")
	}
	if cfg.Logging.MaxSizeMB != 128 {
		t.Fatalf("Logging.MaxSizeMB = %d, want 128", cfg.Logging.MaxSizeMB)
	}
	if cfg.Logging.StaticFields["cluster"] != "test" {
		t.Fatalf("Logging.StaticFields[cluster] = %q, want test", cfg.Logging.StaticFields["cluster"])
	}
	if cfg.Scheduler.BatchSchedulerInterval != 0 {
		t.Fatalf("BatchSchedulerInterval = %v, want 0", cfg.Scheduler.BatchSchedulerInterval)
	}
	if cfg.Scheduler.StaleJobRecoveryInterval != 45*time.Second {
		t.Fatalf("StaleJobRecoveryInterval = %v, want 45s", cfg.Scheduler.StaleJobRecoveryInterval)
	}
	if cfg.GitLab.Domain != "https://gitlab.example.com" {
		t.Fatalf("GitLab.Domain = %q", cfg.GitLab.Domain)
	}
	if cfg.GitLab.Token != "glpat-test" {
		t.Fatalf("GitLab.Token = %q", cfg.GitLab.Token)
	}
}

func TestLoadFromEnv_ParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		errContains string
	}{
		{name: "bool", key: "PLOYD_LOG_JSON", value: "nope", errContains: "PLOYD_LOG_JSON"},
		{name: "int", key: "PLOYD_LOG_MAX_SIZE_MB", value: "x", errContains: "PLOYD_LOG_MAX_SIZE_MB"},
		{name: "duration", key: "PLOYD_HTTP_READ_TIMEOUT", value: "1lightyear", errContains: "PLOYD_HTTP_READ_TIMEOUT"},
		{name: "json", key: "PLOYD_LOG_STATIC_FIELDS", value: "{oops}", errContains: "PLOYD_LOG_STATIC_FIELDS"},
		{name: "listen", key: "PLOYD_HTTP_LISTEN", value: "127.0.0.1", errContains: "http.listen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvForLoadFromEnv(t)
			t.Setenv(tt.key, tt.value)
			_, err := config.LoadFromEnv()
			if err == nil {
				t.Fatal("LoadFromEnv() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func clearEnvForLoadFromEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PLOY_DB_DSN",
		"PLOY_AUTH_SECRET",
		"PLOY_OBJECTSTORE_ENDPOINT",
		"PLOY_OBJECTSTORE_BUCKET",
		"PLOY_OBJECTSTORE_ACCESS_KEY",
		"PLOY_OBJECTSTORE_SECRET_KEY",
		"PLOY_OBJECTSTORE_SECURE",
		"PLOY_OBJECTSTORE_REGION",
		"PLOY_GITLAB_DOMAIN",
		"PLOY_GITLAB_TOKEN",
		"PLOYD_HTTP_LISTEN",
		"PLOYD_HTTP_READ_TIMEOUT",
		"PLOYD_HTTP_WRITE_TIMEOUT",
		"PLOYD_HTTP_IDLE_TIMEOUT",
		"PLOYD_METRICS_LISTEN",
		"PLOYD_ADMIN_SOCKET",
		"PLOYD_ADMIN_LISTEN",
		"PLOYD_PKI_BUNDLE_DIR",
		"PLOYD_PKI_CERTIFICATE",
		"PLOYD_PKI_KEY",
		"PLOYD_PKI_RENEW_BEFORE",
		"PLOYD_PKI_CA_ENDPOINT",
		"PLOYD_SCHEDULER_HOUSEKEEPING_INTERVAL",
		"PLOYD_SCHEDULER_DISK_PRUNE_INTERVAL",
		"PLOYD_SCHEDULER_TTL",
		"PLOYD_SCHEDULER_TTL_INTERVAL",
		"PLOYD_SCHEDULER_DROP_PARTITIONS",
		"PLOYD_SCHEDULER_BATCH_SCHEDULER_INTERVAL",
		"PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL",
		"PLOYD_SCHEDULER_NODE_STALE_AFTER",
		"PLOYD_LOG_LEVEL",
		"PLOYD_LOG_FILE",
		"PLOYD_LOG_MAX_SIZE_MB",
		"PLOYD_LOG_MAX_BACKUPS",
		"PLOYD_LOG_MAX_AGE_DAYS",
		"PLOYD_LOG_JSON",
		"PLOYD_LOG_STATIC_FIELDS",
		"PLOYD_AUTH_BEARER_TOKENS_ENABLED",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}
