package app

import (
	"fmt"
	"os"
	"testing"
)

// Ensure CLI package tests never touch the real ~/.config/ploy.
func TestMain(m *testing.M) {
	tmpHome, err := os.MkdirTemp("", "ploy-testcfg-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "cmd/ploy tests: create temp config home: %v\n", err)
		os.Exit(2)
	}
	_ = os.Setenv("PLOY_CONFIG_HOME", tmpHome)
	_ = os.Setenv("PLOY_SERVER_URL", "http://127.0.0.1:8080")
	_ = os.Setenv("PLOY_AUTH_TOKEN", "test-token")

	code := m.Run()

	// Best-effort cleanup of the temp config home created for this test package.
	_ = os.RemoveAll(tmpHome)
	os.Exit(code)
}
