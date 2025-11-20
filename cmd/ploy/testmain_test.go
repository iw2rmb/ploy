package main

import (
	"os"
	"path/filepath"
	"testing"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
)

// Ensure CLI package tests never touch the real ~/.config/ploy.
func TestMain(m *testing.M) {
	if os.Getenv("PLOY_CONFIG_HOME") == "" {
		tmp, _ := os.MkdirTemp("", "ploy-testcfg-*")
		_ = os.Setenv("PLOY_CONFIG_HOME", tmp)
		_ = os.Setenv("XDG_CONFIG_HOME", "")
	}
	// Ensure an isolated default descriptor exists for all CLI tests.
	tmpHome := os.Getenv("PLOY_CONFIG_HOME")
	if tmpHome != "" {
		_, _ = cliconfig.SaveDescriptor(cliconfig.Descriptor{
			ClusterID:       cliconfig.ClusterID("test-cluster"),
			Address:         "http://127.0.0.1:8080",
			SSHIdentityPath: "/tmp/id_rsa",
			Token:           "test-token",
		})
		_ = cliconfig.SetDefault(cliconfig.ClusterID("test-cluster"))
	}

	code := m.Run()

	// Best-effort cleanup.
	if dir := os.Getenv("PLOY_CONFIG_HOME"); dir != "" {
		_ = os.RemoveAll(filepath.Join(dir, "clusters"))
	}
	os.Exit(code)
}
