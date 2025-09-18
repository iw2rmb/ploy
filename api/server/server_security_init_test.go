package server

import (
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/iw2rmb/ploy/internal/config"
)

// Verifies security handler initialization resolves storage via config service preference
func TestInitializeSecurityHandler_PrefersServiceStorage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	valid := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(valid, []byte("storage:\n  provider: memory\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Create service from valid file, but pass a ControllerConfig with missing path
	svc, err := cfg.New(cfg.WithFile(valid))
	if err != nil {
		t.Fatalf("svc: %v", err)
	}
	conf := &ControllerConfig{StorageConfigPath: filepath.Join(dir, "missing.yaml")}

	securityHandler, recipesHandler, err := initializeSecurityHandlers(conf, svc)
	if err != nil {
		t.Fatalf("security init: %v", err)
	}
	if securityHandler == nil {
		t.Fatalf("handler nil")
	}
	if recipesHandler == nil {
		t.Fatalf("recipes handler nil")
	}
}
