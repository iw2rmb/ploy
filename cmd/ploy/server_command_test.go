package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleServerRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServer(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing server subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy server") {
		t.Fatalf("expected server usage output, got: %q", out)
	}
}

func TestHandleServerDeployRequiresAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServerDeploy(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: ploy server deploy") {
		t.Fatalf("expected deploy usage output, got: %q", buf.String())
	}
}

func TestHandleServerDeployRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServerDeploy([]string{"--address", "1.2.3.4", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}
