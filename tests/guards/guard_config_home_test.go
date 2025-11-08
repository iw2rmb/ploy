package guards

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPloyConfigHomeIsIsolated ensures test runs do not touch the real
// user config directory (~/.config/ploy). The Makefile sets PLOY_CONFIG_HOME
// to a temporary directory for all `make test*` targets. If this guard fails,
// tests are running in an unsafe environment.
func TestPloyConfigHomeIsIsolated(t *testing.T) {
	cfg := os.Getenv("PLOY_CONFIG_HOME")
	if cfg == "" {
		t.Fatalf("PLOY_CONFIG_HOME must be set for tests")
	}
	home := os.Getenv("HOME")
	if home == "" {
		// On some CI systems HOME may be unset; nothing more to assert.
		return
	}
	real := filepath.Join(home, ".config", "ploy")
	// Normalize paths for comparison.
	absCfg, _ := filepath.Abs(cfg)
	absReal, _ := filepath.Abs(real)
	if absCfg == absReal {
		t.Fatalf("unsafe test environment: PLOY_CONFIG_HOME (%s) equals real ~/.config/ploy (%s)", absCfg, absReal)
	}
}
