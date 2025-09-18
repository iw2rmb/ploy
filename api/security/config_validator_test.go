package security

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnvNVDEOverrides(t *testing.T) {
	t.Setenv("NVD_ENABLED", "false")
	t.Setenv("NVD_API_KEY", "test-key")
	t.Setenv("NVD_BASE_URL", "https://example.com/nvd")
	t.Setenv("NVD_TIMEOUT_MS", "1500")

	cfg := LoadConfigFromEnv()
	if cfg.NVD.Enabled {
		t.Fatalf("expected NVD disabled via env override")
	}
	if cfg.NVD.APIKey != "test-key" {
		t.Fatalf("unexpected API key: %s", cfg.NVD.APIKey)
	}
	if cfg.NVD.BaseURL != "https://example.com/nvd" {
		t.Fatalf("unexpected base URL: %s", cfg.NVD.BaseURL)
	}
	if cfg.NVD.Timeout != 1500*time.Millisecond {
		t.Fatalf("unexpected timeout: %v", cfg.NVD.Timeout)
	}
}

func TestProductionConfigUsesDefaults(t *testing.T) {
	t.Setenv("PLOY_ENVIRONMENT", "production")
	t.Setenv("NVD_ENABLED", "")
	cfg := LoadConfigFromEnv()
	if !cfg.NVD.Enabled {
		t.Fatalf("expected NVD enabled by default in production config")
	}
	if cfg.NVD.BaseURL == "" {
		t.Fatalf("expected default base URL in production config")
	}
}
