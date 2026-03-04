package clienv

import (
	"testing"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
)

func UseServerDescriptor(t testing.TB, baseURL string) {
	t.Helper()
	cfgHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := cliconfig.SaveDescriptor(cliconfig.Descriptor{ClusterID: cliconfig.ClusterID("test-cluster"), Address: baseURL}); err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if err := cliconfig.SetDefault(cliconfig.ClusterID("test-cluster")); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
}
