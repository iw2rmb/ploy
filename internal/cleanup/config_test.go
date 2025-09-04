package cleanup

import (
    "os"
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
    os.Unsetenv("NOMAD_ADDR")
    cfg := DefaultTTLConfig()
    if cfg.NomadAddr == "" {
        t.Fatalf("expected NomadAddr to have a default value, got empty")
    }
}

