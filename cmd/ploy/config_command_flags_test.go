package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleConfigRequiresSubcommand verifies that the config command rejects
// invocations without a subcommand and displays usage information.
func TestHandleConfigRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfig(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing config subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config") {
		t.Fatalf("expected config usage output, got: %q", out)
	}
}

// TestHandleConfigUnknownSubcommand ensures that unknown config subcommands
// are rejected with an appropriate error message.
func TestHandleConfigUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfig([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown config subcommand")
	}
	if !strings.Contains(err.Error(), "unknown config subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabRequiresSubcommand verifies that the 'config gitlab'
// command requires a subcommand (show, set, validate) and displays usage.
func TestHandleConfigGitLabRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLab(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing gitlab subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy config gitlab") {
		t.Fatalf("expected gitlab usage output, got: %q", out)
	}
}

// TestHandleConfigGitLabUnknownSubcommand ensures that unknown gitlab subcommands
// are rejected with an appropriate error message.
func TestHandleConfigGitLabUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLab([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown gitlab subcommand")
	}
	if !strings.Contains(err.Error(), "unknown gitlab subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabShowRejectsExtraArgs ensures that the 'show' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigGitLabShowRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabShow([]string{"extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabSetRequiresFile verifies that the 'set' subcommand
// requires the --file flag to be specified.
func TestHandleConfigGitLabSetRequiresFile(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabSetRejectsExtraArgs ensures that the 'set' subcommand
// rejects unexpected positional arguments after flags are parsed.
func TestHandleConfigGitLabSetRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabSet([]string{"--file", "test.json", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabValidateRequiresFile verifies that the 'validate' subcommand
// requires the --file flag to be specified.
func TestHandleConfigGitLabValidateRequiresFile(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabValidate(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleConfigGitLabValidateRejectsExtraArgs ensures that the 'validate' subcommand
// rejects unexpected positional arguments after flags are parsed.
func TestHandleConfigGitLabValidateRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleConfigGitLabValidate([]string{"--file", "test.json", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateGitLabConfigURLRules tests the validation rules for GitLab domain URLs,
// ensuring that proper schemes (http/https) and hosts are required.
func TestValidateGitLabConfigURLRules(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *gitLabConfigPayload
		wantErr string
	}{
		{name: "no scheme", cfg: &gitLabConfigPayload{Domain: "gitlab.com", Token: "x"}, wantErr: "domain must use http or https scheme"},
		{name: "ftp scheme", cfg: &gitLabConfigPayload{Domain: "ftp://gitlab.com", Token: "x"}, wantErr: "domain must use http or https scheme"},
		{name: "empty host", cfg: &gitLabConfigPayload{Domain: "https://", Token: "x"}, wantErr: "domain host is required"},
		{name: "http allowed", cfg: &gitLabConfigPayload{Domain: "http://gitlab.local", Token: "x"}, wantErr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitLabConfig(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

// TestValidateGitLabConfig verifies the validation logic for GitLab configuration
// payloads, testing nil configs, missing fields, and whitespace-only values.
func TestValidateGitLabConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *gitLabConfigPayload
		expectErr bool
		errMsg    string
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectErr: true,
			errMsg:    "configuration is nil",
		},
		{
			name:      "valid config",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: "glpat-123"},
			expectErr: false,
		},
		{
			name:      "missing domain",
			cfg:       &gitLabConfigPayload{Domain: "", Token: "glpat-123"},
			expectErr: true,
			errMsg:    "domain is required",
		},
		{
			name:      "missing token",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: ""},
			expectErr: true,
			errMsg:    "token is required",
		},
		{
			name:      "whitespace domain",
			cfg:       &gitLabConfigPayload{Domain: "   ", Token: "glpat-123"},
			expectErr: true,
			errMsg:    "domain is required",
		},
		{
			name:      "whitespace token",
			cfg:       &gitLabConfigPayload{Domain: "https://gitlab.com", Token: "   "},
			expectErr: true,
			errMsg:    "token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitLabConfig(tt.cfg)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
