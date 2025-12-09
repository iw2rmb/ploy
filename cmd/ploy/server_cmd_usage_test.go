package main

import (
	"bytes"
	"strings"
	"testing"
)

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
