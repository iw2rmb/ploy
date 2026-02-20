package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
)

// captureStdout runs fn and captures everything it writes to os.Stdout.
// Returns the captured output as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// useServerDescriptor configures a temporary default cluster descriptor that points
// to the provided base URL. Tests should call this instead of setting legacy
// environment overrides.
func useServerDescriptor(t testing.TB, baseURL string) {
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
