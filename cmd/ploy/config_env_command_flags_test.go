package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestHandleConfigEnv_Routing verifies top-level routing: missing subcommand,
// unknown subcommand, help flags, ls alias, and config→env dispatch.
func TestHandleConfigEnv_Routing(t *testing.T) {
	tests := []struct {
		name            string
		fn              func([]string, io.Writer) error
		args            []string
		wantErr         bool
		wantErrContains string
		wantOutContains string
	}{
		{
			name:            "missing subcommand",
			fn:              handleConfigEnv,
			args:            nil,
			wantErr:         true,
			wantOutContains: "Usage: ploy config env",
		},
		{
			name:            "unknown subcommand",
			fn:              handleConfigEnv,
			args:            []string{"unknown"},
			wantErr:         true,
			wantErrContains: "unknown env subcommand",
		},
		{
			name:            "help flag --help",
			fn:              handleConfigEnv,
			args:            []string{"--help"},
			wantOutContains: "Usage: ploy config env",
		},
		{
			name:            "help flag -h",
			fn:              handleConfigEnv,
			args:            []string{"-h"},
			wantOutContains: "Usage: ploy config env",
		},
		{
			name:            "ls alias routes to list",
			fn:              handleConfigEnv,
			args:            []string{"ls", "extra"},
			wantErr:         true,
			wantErrContains: "unexpected arguments:",
		},
		{
			name:            "config env routing",
			fn:              handleConfig,
			args:            []string{"env"},
			wantErr:         true,
			wantErrContains: "env subcommand required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tt.fn(tt.args, buf)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
			}
			out := buf.String()
			if tt.wantOutContains != "" && !strings.Contains(out, tt.wantOutContains) {
				t.Fatalf("expected output containing %q, got: %q", tt.wantOutContains, out)
			}
		})
	}
}

// TestHandleConfigEnvShow_FlagValidation verifies flag parsing for the 'show' subcommand.
func TestHandleConfigEnvShow_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", nil, "--key is required"},
		{"empty key", []string{"--key", ""}, "--key is required"},
		{"extra args", []string{"--key", "FOO", "extra"}, "unexpected arguments:"},
		{"invalid from", []string{"--key", "FOO", "--from", "bogus"}, "invalid --from target"},
		{"empty from", []string{"--key", "FOO", "--from", ""}, "--from value cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigEnvShow(tt.args, buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
			}
		})
	}
}

// TestHandleConfigEnvSet_FlagValidation verifies flag parsing for the 'set' subcommand.
func TestHandleConfigEnvSet_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", []string{"--value", "test"}, "--key is required"},
		{"empty key", []string{"--key", "", "--value", "test"}, "--key is required"},
		{"missing value and file", []string{"--key", "FOO"}, "either --value or --file is required"},
		{"value and file exclusive", []string{"--key", "FOO", "--value", "bar", "--file", "test.txt"}, "--value and --file are mutually exclusive"},
		{"extra args", []string{"--key", "FOO", "--value", "bar", "extra"}, "unexpected arguments:"},
		{"invalid on selector", []string{"--key", "FOO", "--value", "bar", "--on", "invalid"}, "invalid --on selector"},
		{"file not found", []string{"--key", "FOO", "--file", "/nonexistent/path/file.txt"}, "read file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigEnvSet(tt.args, buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
			}
		})
	}
}

// TestHandleConfigEnvUnset_FlagValidation verifies flag parsing for the 'unset' subcommand.
func TestHandleConfigEnvUnset_FlagValidation(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantErrContains string
	}{
		{"missing key", nil, "--key is required"},
		{"empty key", []string{"--key", ""}, "--key is required"},
		{"extra args", []string{"--key", "FOO", "extra"}, "unexpected arguments:"},
		{"invalid from", []string{"--key", "FOO", "--from", "bogus"}, "invalid --from target"},
		{"empty from", []string{"--key", "FOO", "--from", ""}, "--from value cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigEnvUnset(tt.args, buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
			}
		})
	}
}

// TestHandleConfigEnvListRejectsExtraArgs ensures that the 'list' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigEnvListRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvList([]string{"extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetValidOnSelectors verifies that all valid --on values are accepted.
func TestHandleConfigEnvSetValidOnSelectors(t *testing.T) {
	validSelectors := []string{"all", "jobs", "server", "nodes", "gates", "steps"}
	for _, sel := range validSelectors {
		t.Run(sel, func(t *testing.T) {
			buf := &bytes.Buffer{}
			// This will fail at resolveControlPlaneHTTP, but selector validation should pass.
			err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", sel}, buf)
			// We expect an error (no server descriptor), but NOT a selector error.
			if err == nil {
				t.Fatalf("expected error (no server descriptor)")
			}
			if strings.Contains(err.Error(), "invalid --on selector") {
				t.Fatalf("selector %q should be valid, got: %v", sel, err)
			}
		})
	}
}

// TestHandleConfigEnvSetOnAllExclusive verifies that --on all cannot be combined with other selectors.
func TestHandleConfigEnvSetOnAllExclusive(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "all then gates", args: []string{"--key", "FOO", "--value", "bar", "--on", "all", "--on", "gates"}},
		{name: "gates then all", args: []string{"--key", "FOO", "--value", "bar", "--on", "gates", "--on", "all"}},
		{name: "all then jobs", args: []string{"--key", "FOO", "--value", "bar", "--on", "all", "--on", "jobs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigEnvSet(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error for --on all combined with other selectors")
			}
			if !strings.Contains(err.Error(), "--on all is exclusive") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestHandleConfigEnvSetMultipleOnSelectors verifies that multiple --on selectors are accepted and deduplicated.
func TestHandleConfigEnvSetMultipleOnSelectors(t *testing.T) {
	buf := &bytes.Buffer{}
	// --on gates --on steps should pass selector validation (will fail at resolveControlPlaneHTTP).
	err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", "gates", "--on", "steps"}, buf)
	if err == nil {
		t.Fatalf("expected error (no server descriptor)")
	}
	if strings.Contains(err.Error(), "invalid --on selector") {
		t.Fatalf("selector should be valid, got: %v", err)
	}
	if strings.Contains(err.Error(), "--on all is exclusive") {
		t.Fatalf("should not trigger all exclusivity, got: %v", err)
	}
}

// TestExpandOnSelector verifies selector expansion and validation.
func TestExpandOnSelector(t *testing.T) {
	tests := []struct {
		name      string
		selector  string
		wantNames []string
		wantErr   string
	}{
		{name: "all expands to four targets", selector: "all", wantNames: []string{"gates", "nodes", "server", "steps"}},
		{name: "jobs expands to gates+steps", selector: "jobs", wantNames: []string{"gates", "steps"}},
		{name: "server single", selector: "server", wantNames: []string{"server"}},
		{name: "nodes single", selector: "nodes", wantNames: []string{"nodes"}},
		{name: "gates single", selector: "gates", wantNames: []string{"gates"}},
		{name: "steps single", selector: "steps", wantNames: []string{"steps"}},
		{name: "invalid selector", selector: "bogus", wantErr: "invalid --on selector"},
		{name: "empty selector", selector: "", wantErr: "invalid --on selector"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := expandOnSelector(tt.selector)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := make([]string, len(targets))
			for i, tgt := range targets {
				got[i] = tgt.String()
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("expected %v, got %v", tt.wantNames, got)
			}
			for i := range got {
				if got[i] != tt.wantNames[i] {
					t.Fatalf("expected %v, got %v", tt.wantNames, got)
				}
			}
		})
	}
}

// TestConfigUsageIncludesEnv verifies that the config command usage includes the env subcommand.
func TestConfigUsageIncludesEnv(t *testing.T) {
	buf := &bytes.Buffer{}
	printConfigUsage(buf)
	out := buf.String()
	if !strings.Contains(out, "env") {
		t.Fatalf("expected 'env' in config usage, got: %q", out)
	}
	if !strings.Contains(out, "Manage global environment variables") {
		t.Fatalf("expected env description in config usage, got: %q", out)
	}
}
