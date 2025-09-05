package server

import (
	"os"
	"path/filepath"
	"testing"
)

// Test_LoadConfigFromEnv_UsesEnvPath verifies the storage config path is taken from env when set
func Test_LoadConfigFromEnv_UsesEnvPath(t *testing.T) {
	t.Setenv("PLOY_STORAGE_CONFIG", "/tmp/ploy/storage.yaml")
	cfg := LoadConfigFromEnv()
	if got, want := cfg.StorageConfigPath, "/tmp/ploy/storage.yaml"; got != want {
		t.Fatalf("StorageConfigPath mismatch: got %q want %q", got, want)
	}
}

// Test_LoadConfigFromEnv_Fallbacks verifies the default fallback path is used when env and external paths are absent
func Test_LoadConfigFromEnv_Fallbacks(t *testing.T) {
	t.Setenv("PLOY_STORAGE_CONFIG", "")

	// Ensure common external locations do not exist in test sandbox
	_ = os.Remove("/etc/ploy/storage/config.yaml")
	_ = os.Remove("/etc/ploy/config.yaml")

	cfg := LoadConfigFromEnv()
	// Expect embedded default path
	if got, want := filepath.Clean(cfg.StorageConfigPath), filepath.Clean("configs/storage-config.yaml"); got != want {
		t.Fatalf("StorageConfigPath mismatch: got %q want %q", got, want)
	}
}
