package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleConfigEnvRequiresSubcommand verifies that the 'config env' command
// requires a subcommand and displays usage information when none is provided.
func TestHandleConfigEnvRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnv(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing env subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config env") {
		t.Fatalf("expected env usage output, got: %q", out)
	}
}

// TestHandleConfigEnvUnknownSubcommand ensures that unknown env subcommands
// are rejected with an appropriate error message.
func TestHandleConfigEnvUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnv([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown env subcommand")
	}
	if !strings.Contains(err.Error(), "unknown env subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvHelpFlag verifies that --help flag displays usage
// and exits without error.
func TestHandleConfigEnvHelpFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "help flag", args: []string{"--help"}},
		{name: "h flag", args: []string{"-h"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigEnv(tt.args, buf)
			if err != nil {
				t.Fatalf("expected no error for help flag, got: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, "Usage: ploy config env") {
				t.Fatalf("expected env usage output, got: %q", out)
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

// TestHandleConfigEnvShowRequiresKey verifies that the 'show' subcommand
// requires the --key flag to be specified.
func TestHandleConfigEnvShowRequiresKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvShow(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --key is missing")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowRejectsExtraArgs ensures that the 'show' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigEnvShowRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "FOO", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowEmptyKey ensures that an empty --key value is rejected.
func TestHandleConfigEnvShowEmptyKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", ""}, buf)
	if err == nil {
		t.Fatalf("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowInvalidFrom verifies that invalid --from values are rejected.
func TestHandleConfigEnvShowInvalidFrom(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "FOO", "--from", "bogus"}, buf)
	if err == nil {
		t.Fatalf("expected error for invalid --from")
	}
	if !strings.Contains(err.Error(), "invalid --from target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvShowEmptyFrom verifies that an empty --from value is rejected.
func TestHandleConfigEnvShowEmptyFrom(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvShow([]string{"--key", "FOO", "--from", ""}, buf)
	if err == nil {
		t.Fatalf("expected error for empty --from")
	}
	if !strings.Contains(err.Error(), "--from value cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetRequiresKey verifies that the 'set' subcommand
// requires the --key flag to be specified.
func TestHandleConfigEnvSetRequiresKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--value", "test"}, buf)
	if err == nil {
		t.Fatalf("expected error when --key is missing")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetRequiresValueOrFile verifies that the 'set' subcommand
// requires either --value or --file to be specified.
func TestHandleConfigEnvSetRequiresValueOrFile(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO"}, buf)
	if err == nil {
		t.Fatalf("expected error when neither --value nor --file is provided")
	}
	if !strings.Contains(err.Error(), "either --value or --file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetValueFileExclusive verifies that --value and --file
// are mutually exclusive.
func TestHandleConfigEnvSetValueFileExclusive(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--file", "test.txt"}, buf)
	if err == nil {
		t.Fatalf("expected error when both --value and --file are provided")
	}
	if !strings.Contains(err.Error(), "--value and --file are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetRejectsExtraArgs ensures that the 'set' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigEnvSetRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetInvalidOnSelector verifies that invalid --on values are rejected.
func TestHandleConfigEnvSetInvalidOnSelector(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO", "--value", "bar", "--on", "invalid"}, buf)
	if err == nil {
		t.Fatalf("expected error for invalid --on selector")
	}
	if !strings.Contains(err.Error(), "invalid --on selector") {
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

// TestHandleConfigEnvUnsetRequiresKey verifies that the 'unset' subcommand
// requires the --key flag to be specified.
func TestHandleConfigEnvUnsetRequiresKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --key is missing")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvUnsetRejectsExtraArgs ensures that the 'unset' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigEnvUnsetRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", "FOO", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvUnsetEmptyKey ensures that an empty --key value is rejected.
func TestHandleConfigEnvUnsetEmptyKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", ""}, buf)
	if err == nil {
		t.Fatalf("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvUnsetInvalidFrom verifies that invalid --from values are rejected.
func TestHandleConfigEnvUnsetInvalidFrom(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", "FOO", "--from", "bogus"}, buf)
	if err == nil {
		t.Fatalf("expected error for invalid --from")
	}
	if !strings.Contains(err.Error(), "invalid --from target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvUnsetEmptyFrom verifies that an empty --from value is rejected.
func TestHandleConfigEnvUnsetEmptyFrom(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvUnset([]string{"--key", "FOO", "--from", ""}, buf)
	if err == nil {
		t.Fatalf("expected error for empty --from")
	}
	if !strings.Contains(err.Error(), "--from value cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetFileNotFound verifies that missing files are detected.
func TestHandleConfigEnvSetFileNotFound(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "FOO", "--file", "/nonexistent/path/file.txt"}, buf)
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigEnvSetEmptyKey ensures that an empty --key value is rejected.
func TestHandleConfigEnvSetEmptyKey(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigEnvSet([]string{"--key", "", "--value", "test"}, buf)
	if err == nil {
		t.Fatalf("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("unexpected error: %v", err)
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

// TestConfigUsageIncludesEnv verifies that the config command usage now
// includes the env subcommand.
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

// TestConfigEnvRouting verifies that 'config env' routes to handleConfigEnv.
func TestConfigEnvRouting(t *testing.T) {
	buf := &bytes.Buffer{}
	// Without arguments, handleConfigEnv returns an error asking for subcommand.
	err := handleConfig([]string{"env"}, buf)
	if err == nil {
		t.Fatalf("expected error for missing env subcommand")
	}
	if !strings.Contains(err.Error(), "env subcommand required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
