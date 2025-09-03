package server

import (
    "os"
    "path/filepath"
    "testing"

    cfg "github.com/iw2rmb/ploy/internal/config"
)

// Ensures initializeDependenciesWithService succeeds using the config service even if
// the file-based path would be invalid.
func TestInitializeDependencies_PrefersConfigService(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    valid := filepath.Join(dir, "config.yaml")
    // Minimal service config using memory provider
    content := []byte("storage:\n  provider: memory\n")
    if err := os.WriteFile(valid, content, 0o644); err != nil {
        t.Fatalf("write: %v", err)
    }

    svc, err := cfg.New(cfg.WithFile(valid))
    if err != nil {
        t.Fatalf("config service init: %v", err)
    }

    // ControllerConfig points to a missing path; service should be preferred
    conf := &ControllerConfig{StorageConfigPath: filepath.Join(dir, "missing.yaml")}
    deps, err := initializeDependenciesWithService(conf, svc)
    if err != nil {
        t.Fatalf("initialize deps with service failed: %v", err)
    }
    if deps == nil {
        t.Fatalf("deps nil")
    }
}

