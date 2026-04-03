package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleConfigHomeRequiresSubcommand verifies that the 'config home' command
// requires a subcommand and displays usage information when none is provided.
func TestHandleConfigHomeRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHome(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing home subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config home") {
		t.Fatalf("expected home usage output, got: %q", out)
	}
}

// TestHandleConfigHomeUnknownSubcommand ensures that unknown home subcommands
// are rejected with an appropriate error message.
func TestHandleConfigHomeUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHome([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown home subcommand")
	}
	if !strings.Contains(err.Error(), "unknown home subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigHomeLsAliasRoutes verifies that 'ls' routes to the list handler.
func TestHandleConfigHomeLsAliasRoutes(t *testing.T) {
	buf := &bytes.Buffer{}
	// 'ls' with unexpected args triggers the same error as 'list' with unexpected args.
	err := handleConfigHome([]string{"ls", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args via ls alias")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigHomeSetRequiresEntry verifies that the 'set' subcommand
// requires the --entry flag.
func TestHandleConfigHomeSetRequiresEntry(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHomeSet([]string{"--section", "mig"}, buf)
	if err == nil {
		t.Fatalf("expected error when --entry is missing")
	}
	if !strings.Contains(err.Error(), "--entry is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigHomeSetRequiresSection verifies that the 'set' subcommand
// requires the --section flag.
func TestHandleConfigHomeSetRequiresSection(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHomeSet([]string{"--entry", "abcdef1:.config/app"}, buf)
	if err == nil {
		t.Fatalf("expected error when --section is missing")
	}
	if !strings.Contains(err.Error(), "--section is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigHomeUnsetRequiresDst verifies that the 'unset' subcommand
// requires the --dst flag.
func TestHandleConfigHomeUnsetRequiresDst(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHomeUnset([]string{"--section", "mig"}, buf)
	if err == nil {
		t.Fatalf("expected error when --dst is missing")
	}
	if !strings.Contains(err.Error(), "--dst is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigHomeUnsetRequiresSection verifies that the 'unset' subcommand
// requires the --section flag.
func TestHandleConfigHomeUnsetRequiresSection(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigHomeUnset([]string{"--dst", ".config/app"}, buf)
	if err == nil {
		t.Fatalf("expected error when --section is missing")
	}
	if !strings.Contains(err.Error(), "--section is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
