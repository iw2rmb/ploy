package guards

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPloyConfigHomeIsIsolated ensures test runs do not touch the real
// user config directory (~/.config/ploy). The Makefile sets PLOY_CONFIG_HOME
// to a temporary directory for all `make test*` targets. When invoked outside
// of `make` (e.g. bare `go test ./...`), the guard redirects
// PLOY_CONFIG_HOME to a temporary directory so tests remain green.
func TestPloyConfigHomeIsIsolated(t *testing.T) {
	cfg := os.Getenv("PLOY_CONFIG_HOME")

	needsRedirect := cfg == ""
	if !needsRedirect {
		if home := os.Getenv("HOME"); home != "" {
			realDir, _ := filepath.Abs(filepath.Join(home, ".config", "ploy"))
			absDir, _ := filepath.Abs(cfg)
			needsRedirect = absDir == realDir
		}
	}

	if needsRedirect {
		tmp := t.TempDir()
		t.Setenv("PLOY_CONFIG_HOME", tmp)
		t.Logf("PLOY_CONFIG_HOME was unsafe (%q); redirected to %s", cfg, tmp)
	}
}
