package main

import (
	"os"
	"path/filepath"
	"testing"
)

// IsolatePloyConfigHome sets a per-test config home and disables
// default marker mutations. Cleans up the clusters dir on exit.
// Use this for nearly all tests that touch descriptors.
func IsolatePloyConfigHome(t *testing.T) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("PLOY_NO_DEFAULT_MUTATION", "1")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(cfgHome, "clusters")) })
	return cfgHome
}

// IsolatePloyConfigHomeAllowDefault sets a per-test config home but
// allows tests to create a default marker within that temp scope.
// It still cleans up the clusters dir afterwards.
func IsolatePloyConfigHomeAllowDefault(t *testing.T) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(cfgHome, "clusters")) })
	return cfgHome
}
