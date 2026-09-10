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
	if cfg.Scheduler.StaleJobRecoveryInterval != 30*time.Second {
		t.Fatalf("StaleJobRecoveryInterval = %v, want 30s", cfg.Scheduler.StaleJobRecoveryInterval)
	}
	if cfg.Scheduler.NodeStaleAfter != time.Minute {
		t.Fatalf("NodeStaleAfter = %v, want 1m", cfg.Scheduler.NodeStaleAfter)
	}
	if cfg.PKI.RenewBefore != time.Hour {
		t.Fatalf("PKI.RenewBefore = %v, want 1h", cfg.PKI.RenewBefore)
	}
}

func TestLoadFromEnv_Overrides(t *testing.T) {
	clearEnvForLoadFromEnv(t)
	t.Setenv("PLOYD_HTTP_LISTEN", "127.0.0.1:18080")
	t.Setenv("PLOYD_METRICS_LISTEN", "127.0.0.1:19100")
	t.Setenv("PLOYD_HTTP_READ_TIMEOUT", "21s")
	t.Setenv("PLOYD_AUTH_BEARER_TOKENS_ENABLED", "false")
	t.Setenv("PLOYD_LOG_LEVEL", "debug")
	t.Setenv("PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL", "45s")
	t.Setenv("PLOYD_SCHEDULER_NODE_STALE_AFTER", "2m")
	t.Setenv("PLOYD_PKI_BUNDLE_DIR", "/var/lib/ploy/pki")
	t.Setenv("PLOYD_PKI_RENEW_BEFORE", "12m")
	t.Setenv("PLOY_GITLAB_DOMAIN", "https://gitlab.example.com")
	t.Setenv("PLOY_GITLAB_TOKEN", "glpat-test")
	t.Setenv("PLOY_SPECS_REPOS", " https://gitlab.example.com/platform/migs.git , ssh://git@gitlab.example.com/team/scenarios.git ")
	t.Setenv("PLOY_NAMED_SPECS_ENVS_ALLOWLIST", " PLOY_CONTAINER_REGISTRY , RELEASE_CHANNEL ")

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
	if cfg.Logging.Level != "debug" {
		t.Fatalf("Logging.Level = %q, want debug", cfg.Logging.Level)
	}
	if cfg.Scheduler.StaleJobRecoveryInterval != 45*time.Second {
		t.Fatalf("StaleJobRecoveryInterval = %v, want 45s", cfg.Scheduler.StaleJobRecoveryInterval)
	}
	if cfg.Scheduler.NodeStaleAfter != 2*time.Minute {
		t.Fatalf("NodeStaleAfter = %v, want 2m", cfg.Scheduler.NodeStaleAfter)
	}
	if cfg.PKI.BundleDir != "/var/lib/ploy/pki" {
		t.Fatalf("PKI.BundleDir = %q, want /var/lib/ploy/pki", cfg.PKI.BundleDir)
	}
	if cfg.PKI.RenewBefore != 12*time.Minute {
		t.Fatalf("PKI.RenewBefore = %v, want 12m", cfg.PKI.RenewBefore)
	}
	if cfg.GitLab.Domain != "https://gitlab.example.com" {
		t.Fatalf("GitLab.Domain = %q", cfg.GitLab.Domain)
	}
	if cfg.GitLab.Token != "glpat-test" {
		t.Fatalf("GitLab.Token = %q", cfg.GitLab.Token)
	}
	if len(cfg.SpecRepos) != 2 || cfg.SpecRepos[0].String() != "https://gitlab.example.com/platform/migs.git" || cfg.SpecRepos[1].String() != "ssh://git@gitlab.example.com/team/scenarios.git" {
		t.Fatalf("SpecRepos = %#v, want two trimmed repositories", cfg.SpecRepos)
	}
	if got := cfg.NamedSpecEnvAllowlist; len(got) != 2 || got[0] != "PLOY_CONTAINER_REGISTRY" || got[1] != "RELEASE_CHANNEL" {
		t.Fatalf("NamedSpecEnvAllowlist = %#v, want two trimmed names", got)
	}
}

func TestLoadFromEnv_SpecReposValidation(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		wantRepos      int
		errContains    string
		errNotContains string
	}{
		{name: "whitespace configures none", value: "   "},
		{name: "SSH without userinfo", value: "ssh://git.example.com/acme/one.git", wantRepos: 1},
		{name: "empty entry", value: "https://git.example.com/acme/one.git, ,https://git.example.com/acme/two.git", errContains: "entry 2 is empty"},
		{name: "duplicate normalized URL", value: "https://git.example.com/acme/one.git,https://git.example.com/acme/one/", errContains: "duplicate repository"},
		{name: "unsupported scheme", value: "git://git.example.com/acme/one.git", errContains: "invalid repository URL"},
		{name: "missing repository path", value: "https://git.example.com", errContains: "invalid repository URL"},
		{name: "SSH password", value: "ssh://git:secret@git.example.com/acme/one.git", errContains: "password-bearing SSH", errNotContains: "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvForLoadFromEnv(t)
			t.Setenv("PLOY_SPECS_REPOS", tt.value)
			cfg, err := config.LoadFromEnv()
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("LoadFromEnv() error = %v", err)
				}
				if len(cfg.SpecRepos) != tt.wantRepos {
					t.Fatalf("SpecRepos = %#v, want %d repositories", cfg.SpecRepos, tt.wantRepos)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("LoadFromEnv() error = %v, want containing %q", err, tt.errContains)
			}
			if tt.errNotContains != "" && strings.Contains(err.Error(), tt.errNotContains) {
				t.Fatalf("LoadFromEnv() error = %q, must not contain %q", err, tt.errNotContains)
			}
		})
	}
}

func TestLoadFromEnv_NamedSpecEnvAllowlistValidation(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		want        []string
		errContains string
	}{
		{name: "whitespace configures none", value: "   "},
		{name: "trimmed names", value: " PLOY_CONTAINER_REGISTRY,RELEASE_CHANNEL ", want: []string{"PLOY_CONTAINER_REGISTRY", "RELEASE_CHANNEL"}},
		{name: "empty entry", value: "PLOY_CONTAINER_REGISTRY, ,RELEASE_CHANNEL", errContains: "entry 2 is empty"},
		{name: "invalid name", value: "PLOY_CONTAINER_REGISTRY,NOT-VALID", errContains: "entry 2 is not an environment variable name"},
		{name: "duplicate name", value: "PLOY_CONTAINER_REGISTRY,PLOY_CONTAINER_REGISTRY", errContains: "duplicate environment variable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvForLoadFromEnv(t)
			t.Setenv("PLOY_NAMED_SPECS_ENVS_ALLOWLIST", tt.value)
			cfg, err := config.LoadFromEnv()
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("LoadFromEnv() error = %v, want containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFromEnv() error = %v", err)
			}
			if len(cfg.NamedSpecEnvAllowlist) != len(tt.want) {
				t.Fatalf("NamedSpecEnvAllowlist = %#v, want %#v", cfg.NamedSpecEnvAllowlist, tt.want)
			}
			for i := range tt.want {
				if cfg.NamedSpecEnvAllowlist[i] != tt.want[i] {
					t.Fatalf("NamedSpecEnvAllowlist = %#v, want %#v", cfg.NamedSpecEnvAllowlist, tt.want)
				}
			}
		})
	}
}

func TestLoadFromEnv_ParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		errContains string
	}{
		{name: "bool", key: "PLOYD_AUTH_BEARER_TOKENS_ENABLED", value: "nope", errContains: "PLOYD_AUTH_BEARER_TOKENS_ENABLED"},
		{name: "duration", key: "PLOYD_HTTP_READ_TIMEOUT", value: "1lightyear", errContains: "PLOYD_HTTP_READ_TIMEOUT"},
		{name: "http listen", key: "PLOYD_HTTP_LISTEN", value: "127.0.0.1", errContains: "http.listen"},
		{name: "metrics listen", key: "PLOYD_METRICS_LISTEN", value: "127.0.0.1:99999", errContains: "metrics.listen"},
		{name: "admin listen", key: "PLOYD_ADMIN_LISTEN", value: "127.0.0.1", errContains: "admin.listen"},
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
		"PLOY_SPECS_REPOS",
		"PLOY_NAMED_SPECS_ENVS_ALLOWLIST",
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
		"PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL",
		"PLOYD_SCHEDULER_NODE_STALE_AFTER",
		"PLOYD_LOG_LEVEL",
		"PLOYD_AUTH_BEARER_TOKENS_ENABLED",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}
