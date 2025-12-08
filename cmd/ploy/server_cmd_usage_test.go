package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleServerRequiresSubcommand verifies that handleServer (legacy path)
// produces cluster-scoped usage strings after the re-rooting migration.
// NOTE: The `ploy server` top-level command has been removed; server deployment
// is now accessible only via `ploy cluster deploy`. This test ensures the
// internal handleServer function still works (for reuse/tests) but prints
// cluster-scoped usage.
func TestHandleServerRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServer(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing server subcommand")
	}
	out := buf.String()
	// After migration, usage should reference `ploy cluster deploy` not `ploy server`.
	if !strings.Contains(out, "Usage: ploy cluster deploy") {
		t.Fatalf("expected cluster deploy usage output, got: %q", out)
	}
}

// TestHandleServerDeployMissingAddress verifies that missing --address flag
// produces cluster-scoped usage output.
func TestHandleServerDeployMissingAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	// Call handleServerDeploy with no args (missing required --address flag).
	err := handleServerDeploy(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing address")
	}
	out := buf.String()
	// Usage should reference cluster deploy path.
	if !strings.Contains(out, "Usage: ploy cluster deploy") {
		t.Fatalf("expected cluster deploy usage in error output, got: %q", out)
	}
}
