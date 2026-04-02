package guards

import (
	"os"
	"testing"
)

// Provide a safe default config home for guard tests when not set by the runner.
func TestMain(m *testing.M) {
	if os.Getenv("PLOY_CONFIG_HOME") == "" {
		if tmp, err := os.MkdirTemp("", "ploy-guard-*"); err == nil {
			_ = os.Setenv("PLOY_CONFIG_HOME", tmp)
		}
	}
	os.Exit(m.Run())
}
