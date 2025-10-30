package main

import (
    "bytes"
    "strings"
    "testing"
)

func TestHandleWorkflowCancelValidatesFlags(t *testing.T) {
	buf := &bytes.Buffer{}
err := handleWorkflowCancel([]string{}, buf)
	if err == nil {
		t.Fatal("expected error for missing run id")
	}
	if !strings.Contains(buf.String(), "Usage: ploy workflow cancel") {
		t.Fatalf("expected cancel usage, got %q", buf.String())
	}
}

func TestHandleWorkflowCancelIsDeprecated(t *testing.T) {
    buf := &bytes.Buffer{}
    err := handleWorkflowCancel([]string{"--run-id", "run-123"}, buf)
    if err == nil {
        t.Fatal("expected deprecation error")
    }
    out := buf.String()
    if !strings.Contains(out, "deprecated") || !strings.Contains(out, "ploy mod cancel") {
        t.Fatalf("expected deprecation guidance, got %q", out)
    }
}
