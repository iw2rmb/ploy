package clienv

import (
	"os"
	"testing"
)

func IsolateConfigHome(t testing.TB) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("PLOY_NO_DEFAULT_MUTATION", "1")
	t.Cleanup(func() { _ = os.RemoveAll(cfgHome) })
	return cfgHome
}

func IsolateConfigHomeAllowDefault(t testing.TB) string {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Cleanup(func() { _ = os.RemoveAll(cfgHome) })
	return cfgHome
}
