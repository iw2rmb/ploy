package cleanup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfigFromEnv verifies environment overrides are applied
func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("PLOY_PREVIEW_TTL", "2h")
	t.Setenv("PLOY_CLEANUP_INTERVAL", "30m")
	t.Setenv("PLOY_MAX_PREVIEW_AGE", "48h")
	t.Setenv("PLOY_CLEANUP_DRY_RUN", "true")
	t.Setenv("NOMAD_ADDR", "http://nomad.example:4646")

	cfg := LoadConfigFromEnv()

	if cfg.PreviewTTL != 2*time.Hour {
		t.Fatalf("expected PreviewTTL=2h, got %v", cfg.PreviewTTL)
	}
	if cfg.CleanupInterval != 30*time.Minute {
		t.Fatalf("expected CleanupInterval=30m, got %v", cfg.CleanupInterval)
	}
	if cfg.MaxAge != 48*time.Hour {
		t.Fatalf("expected MaxAge=48h, got %v", cfg.MaxAge)
	}
	if !cfg.DryRun {
		t.Fatalf("expected DryRun=true, got %v", cfg.DryRun)
	}
	if cfg.NomadAddr != "http://nomad.example:4646" {
		t.Fatalf("expected NomadAddr=http://nomad.example:4646, got %s", cfg.NomadAddr)
	}
}

// TestDefaultTTLConfigUsesEnvDefault confirms DefaultTTLConfig respects default when env unset
func TestDefaultTTLConfigUsesEnvDefault(t *testing.T) {
	_ = os.Unsetenv("NOMAD_ADDR")
	cfg := DefaultTTLConfig()
	if cfg.NomadAddr == "" {
		t.Fatalf("expected NomadAddr to have a default value, got empty")
	}
}

// TestConfigManagerLoadConfigCreatesFileWhenMissing ensures a default config is written when none exists
func TestConfigManagerLoadConfigCreatesFileWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "cleanup", "config.json")

	cm := NewConfigManager(configPath)

	cfg, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg == nil {
		t.Fatalf("expected non-nil config from LoadConfig")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("expected config file to be created, got error: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected config file to have contents")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read created config file: %v", err)
	}

	var persisted TTLConfig
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("failed to unmarshal persisted config: %v", err)
	}

	if persisted.PreviewTTL != cfg.PreviewTTL {
		t.Fatalf("expected persisted PreviewTTL=%v, got %v", cfg.PreviewTTL, persisted.PreviewTTL)
	}
	if persisted.CleanupInterval != cfg.CleanupInterval {
		t.Fatalf("expected persisted CleanupInterval=%v, got %v", cfg.CleanupInterval, persisted.CleanupInterval)
	}
	if persisted.MaxAge != cfg.MaxAge {
		t.Fatalf("expected persisted MaxAge=%v, got %v", cfg.MaxAge, persisted.MaxAge)
	}
}

// TestConfigManagerUpdateConfigAppliesValidation verifies updates are normalized and persisted
func TestConfigManagerUpdateConfigAppliesValidation(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")

	cm := NewConfigManager(configPath)
	if _, err := cm.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	updates := map[string]interface{}{
		"preview_ttl":      "10s",
		"cleanup_interval": "2m",
		"max_age":          "30s",
		"dry_run":          true,
		"nomad_addr":       "http://nomad.internal:4646",
	}

	if err := cm.UpdateConfig(updates); err != nil {
		t.Fatalf("UpdateConfig returned error: %v", err)
	}

	got := cm.GetConfig()
	if got.PreviewTTL != time.Minute {
		t.Fatalf("expected PreviewTTL normalized to 1m, got %v", got.PreviewTTL)
	}
	if got.CleanupInterval != 5*time.Minute {
		t.Fatalf("expected CleanupInterval normalized to 5m, got %v", got.CleanupInterval)
	}
	if got.MaxAge != 2*time.Minute {
		t.Fatalf("expected MaxAge normalized to 2m, got %v", got.MaxAge)
	}
	if !got.DryRun {
		t.Fatalf("expected DryRun=true after update")
	}
	if got.NomadAddr != "http://nomad.internal:4646" {
		t.Fatalf("expected NomadAddr to persist override, got %s", got.NomadAddr)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed reading updated config: %v", err)
	}
	var persisted TTLConfig
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("failed to unmarshal persisted config: %v", err)
	}
	if persisted != *got {
		t.Fatalf("persisted config mismatch: %#v vs %#v", persisted, *got)
	}
}

// TestConfigManagerLoadConfigAppliesDefaults ensures zero values are replaced with defaults
func TestConfigManagerLoadConfigAppliesDefaults(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")

	original := TTLConfig{}
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cm := NewConfigManager(configPath)
	cfg, err := cm.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	defaults := DefaultTTLConfig()
	if cfg.PreviewTTL != defaults.PreviewTTL {
		t.Fatalf("expected PreviewTTL default %v, got %v", defaults.PreviewTTL, cfg.PreviewTTL)
	}
	if cfg.CleanupInterval != defaults.CleanupInterval {
		t.Fatalf("expected CleanupInterval default %v, got %v", defaults.CleanupInterval, cfg.CleanupInterval)
	}
	if cfg.MaxAge != defaults.MaxAge {
		t.Fatalf("expected MaxAge default %v, got %v", defaults.MaxAge, cfg.MaxAge)
	}
	if cfg.NomadAddr != defaults.NomadAddr {
		t.Fatalf("expected NomadAddr default %s, got %s", defaults.NomadAddr, cfg.NomadAddr)
	}
}
