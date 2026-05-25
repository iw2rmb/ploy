package configure

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestHandleConfigRouting(t *testing.T) {
	tests := []struct {
		name    string
		fn      func([]string, io.Writer) error
		args    []string
		wantErr string
		wantOut string
	}{
		{
			name:    "config requires subcommand",
			fn:      Handle,
			wantOut: "Usage: ploy config",
		},
		{
			name:    "config rejects unknown subcommand",
			fn:      Handle,
			args:    []string{"unknown"},
			wantErr: "unknown config subcommand",
		},
		{
			name:    "config rejects removed home subcommand",
			fn:      Handle,
			args:    []string{"home"},
			wantErr: "unknown config subcommand",
		},
		{
			name:    "config gitlab requires subcommand",
			fn:      handleConfigGitLab,
			wantOut: "Usage: ploy config gitlab",
		},
		{
			name:    "config gitlab rejects unknown subcommand",
			fn:      handleConfigGitLab,
			args:    []string{"unknown"},
			wantErr: "unknown gitlab subcommand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tt.fn(tt.args, buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
			if tt.wantOut != "" {
				assertx.Contains(t, buf.String(), tt.wantOut)
			}
		})
	}
}

// TestHandleConfigGitLabShowRejectsExtraArgs ensures that the 'show' subcommand
// rejects unexpected positional arguments.
func TestHandleConfigGitLabShowRejectsExtraArgs(t *testing.T) {
	clienv.RunExpectError(t, handleConfigGitLabShow, []string{"extra"}, "unexpected arguments:")
}

// TestHandleConfigGitLabSetRequiresFile verifies that the 'set' subcommand
// requires the --file flag to be specified.
func TestHandleConfigGitLabSetRequiresFile(t *testing.T) {
	clienv.RunExpectError(t, handleConfigGitLabSet, nil, "--file is required")
}

// TestHandleConfigGitLabSetRejectsExtraArgs ensures that the 'set' subcommand
// rejects unexpected positional arguments after flags are parsed.
func TestHandleConfigGitLabSetRejectsExtraArgs(t *testing.T) {
	clienv.RunExpectError(t, handleConfigGitLabSet,
		[]string{"--file", "test.json", "extra"}, "unexpected arguments:")
}

// TestHandleConfigGitLabValidateRequiresFile verifies that the 'validate' subcommand
// requires the --file flag to be specified.
func TestHandleConfigGitLabValidateRequiresFile(t *testing.T) {
	clienv.RunExpectError(t, handleConfigGitLabValidate, nil, "--file is required")
}

// TestHandleConfigGitLabValidateRejectsExtraArgs ensures that the 'validate' subcommand
// rejects unexpected positional arguments after flags are parsed.
func TestHandleConfigGitLabValidateRejectsExtraArgs(t *testing.T) {
	clienv.RunExpectError(t, handleConfigGitLabValidate,
		[]string{"--file", "test.json", "extra"}, "unexpected arguments:")
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
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
