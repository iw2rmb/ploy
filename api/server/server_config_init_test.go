package server

import (
    "os"
    "path/filepath"
    "testing"
)

// Verifies NewServer initializes configService and unified storage resolution works
func TestServer_InitializesConfigServiceAndResolvesStorage(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    cfgPath := filepath.Join(dir, "config.yaml")
    // minimal storage config for memory provider
    content := []byte("storage:\n  provider: memory\n")
    if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
        t.Fatalf("failed to write config: %v", err)
    }

    srv, err := NewServer(&ControllerConfig{StorageConfigPath: cfgPath})
    if err != nil {
        t.Fatalf("NewServer error: %v", err)
    }
    if srv.configService == nil {
        t.Fatalf("expected configService to be initialized")
    }

    st, err := srv.resolveUnifiedStorage()
    if err != nil || st == nil {
        t.Fatalf("resolveUnifiedStorage failed: %v", err)
    }
}

