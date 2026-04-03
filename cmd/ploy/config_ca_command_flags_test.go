package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleConfigCARequiresSubcommand verifies that the 'config ca' command
// requires a subcommand and displays usage information when none is provided.
func TestHandleConfigCARequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCA(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing ca subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config ca") {
		t.Fatalf("expected ca usage output, got: %q", out)
	}
}

// TestHandleConfigCAUnknownSubcommand ensures that unknown ca subcommands
// are rejected with an appropriate error message.
func TestHandleConfigCAUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCA([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown ca subcommand")
	}
	if !strings.Contains(err.Error(), "unknown ca subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCALsAliasRoutes verifies that 'ls' routes to the list handler.
func TestHandleConfigCALsAliasRoutes(t *testing.T) {
	buf := &bytes.Buffer{}
	// 'ls' with unexpected args triggers the same error as 'list' with unexpected args.
	err := handleConfigCA([]string{"ls", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args via ls alias")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCASetRequiresHash verifies that the 'set' subcommand
// requires the --hash flag.
func TestHandleConfigCASetRequiresHash(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCASet([]string{"--section", "mig"}, buf)
	if err == nil {
		t.Fatalf("expected error when --hash is missing")
	}
	if !strings.Contains(err.Error(), "--hash is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCASetRequiresSection verifies that the 'set' subcommand
// requires the --section flag.
func TestHandleConfigCASetRequiresSection(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCASet([]string{"--hash", "abcdef1234567"}, buf)
	if err == nil {
		t.Fatalf("expected error when --section is missing")
	}
	if !strings.Contains(err.Error(), "--section is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCAUnsetRequiresHash verifies that the 'unset' subcommand
// requires the --hash flag.
func TestHandleConfigCAUnsetRequiresHash(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCAUnset([]string{"--section", "mig"}, buf)
	if err == nil {
		t.Fatalf("expected error when --hash is missing")
	}
	if !strings.Contains(err.Error(), "--hash is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCAUnsetRequiresSection verifies that the 'unset' subcommand
// requires the --section flag.
func TestHandleConfigCAUnsetRequiresSection(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigCAUnset([]string{"--hash", "abcdef1234567"}, buf)
	if err == nil {
		t.Fatalf("expected error when --section is missing")
	}
	if !strings.Contains(err.Error(), "--section is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigCASetRejectsInvalidHash verifies that the 'set' subcommand
// applies Hydra parser validation and rejects non-hex hashes locally.
func TestHandleConfigCASetRejectsInvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{name: "uppercase hex", hash: "ABCDEF1234567"},
		{name: "too short", hash: "abc12"},
		{name: "non-hex chars", hash: "ghijklm1234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCASet([]string{"--hash", tt.hash, "--section", "mig"}, buf)
			if err == nil {
				t.Fatalf("expected error for invalid hash %q", tt.hash)
			}
			if !strings.Contains(err.Error(), "ca entry") {
				t.Fatalf("expected Hydra parser error, got: %v", err)
			}
		})
	}
}

// TestHandleConfigCASetRejectsInvalidSection verifies that the 'set' subcommand
// validates Hydra section names locally before making a network request.
func TestHandleConfigCASetRejectsInvalidSection(t *testing.T) {
	for _, s := range []string{"unknown", "server", ""} {
		buf := &bytes.Buffer{}
		args := []string{"--hash", "abcdef1234567", "--section", s}
		if s == "" {
			// Empty section is caught by the required-flag check.
			continue
		}
		err := handleConfigCASet(args, buf)
		if err == nil {
			t.Fatalf("expected error for invalid section %q", s)
		}
		if !strings.Contains(err.Error(), "invalid hydra section") {
			t.Fatalf("expected hydra section validation error, got: %v", err)
		}
	}
}

// TestHandleConfigCAUnsetRejectsInvalidSection verifies that the 'unset' subcommand
// validates Hydra section names locally before making a network request.
func TestHandleConfigCAUnsetRejectsInvalidSection(t *testing.T) {
	for _, s := range []string{"unknown", "server"} {
		buf := &bytes.Buffer{}
		err := handleConfigCAUnset([]string{"--hash", "abcdef1234567", "--section", s}, buf)
		if err == nil {
			t.Fatalf("expected error for invalid section %q", s)
		}
		if !strings.Contains(err.Error(), "invalid hydra section") {
			t.Fatalf("expected hydra section validation error, got: %v", err)
		}
	}
}

// TestHandleConfigCAUnsetRejectsInvalidHash verifies that the 'unset' subcommand
// applies Hydra parser validation and rejects non-hex hashes locally.
func TestHandleConfigCAUnsetRejectsInvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{name: "uppercase hex", hash: "ABCDEF1234567"},
		{name: "too short", hash: "abc12"},
		{name: "non-hex chars", hash: "ghijklm1234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCAUnset([]string{"--hash", tt.hash, "--section", "mig"}, buf)
			if err == nil {
				t.Fatalf("expected error for invalid hash %q", tt.hash)
			}
			if !strings.Contains(err.Error(), "ca entry") {
				t.Fatalf("expected Hydra parser error, got: %v", err)
			}
		})
	}
}
