package main

import (
	"fmt"
	"os"
	"testing"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
)

// Ensure CLI package tests never touch the real ~/.config/ploy.
func TestMain(m *testing.M) {
	tmpHome, err := os.MkdirTemp("", "ploy-testcfg-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "cmd/ploy tests: create temp config home: %v\n", err)
		os.Exit(2)
	}
	_ = os.Setenv("PLOY_CONFIG_HOME", tmpHome)
	_ = os.Setenv("XDG_CONFIG_HOME", "")
	// Ensure an isolated default descriptor exists for all CLI tests.
	_, _ = cliconfig.SaveDescriptor(cliconfig.Descriptor{
		ClusterID:       cliconfig.ClusterID("test-cluster"),
		Address:         "http://127.0.0.1:8080",
		SSHIdentityPath: "/tmp/id_rsa",
		Token:           "test-token",
	})
	_ = cliconfig.SetDefault(cliconfig.ClusterID("test-cluster"))

	code := m.Run()

	// Best-effort cleanup of the temp config home created for this test package.
	_ = os.RemoveAll(tmpHome)
	os.Exit(code)
}
