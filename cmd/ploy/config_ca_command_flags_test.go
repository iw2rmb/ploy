package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestHandleConfigCA_SubcommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		wantOut string // substring expected in stdout (usage output)
	}{
		{
			name:    "requires subcommand",
			args:    nil,
			wantErr: "ca subcommand required",
			wantOut: "Usage: ploy config ca",
		},
		{
			name:    "unknown subcommand rejected",
			args:    []string{"unknown"},
			wantErr: "unknown ca subcommand",
		},
		{
			name:    "ls alias routes to list",
			args:    []string{"ls", "extra"},
			wantErr: "unexpected arguments:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCA(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Fatalf("stdout = %q, want containing %q", buf.String(), tt.wantOut)
			}
		})
	}
}

func TestHandleConfigCASet_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing hash and file",
			args:    []string{"--section", "mig"},
			wantErr: "either --hash or --file is required",
		},
		{
			name:    "missing section",
			args:    []string{"--hash", "abcdef1234567"},
			wantErr: "--section is required",
		},
		{
			name:    "invalid hash uppercase",
			args:    []string{"--hash", "ABCDEF1234567", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid hash too short",
			args:    []string{"--hash", "abc12", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid hash non-hex",
			args:    []string{"--hash", "ghijklm1234567", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid section unknown",
			args:    []string{"--hash", "abcdef1234567", "--section", "unknown"},
			wantErr: "invalid config ca section",
		},
		{
			name:    "invalid section server",
			args:    []string{"--hash", "abcdef1234567", "--section", "server"},
			wantErr: "invalid config ca section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCASet(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHandleConfigCAUnset_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing hash",
			args:    []string{"--section", "mig"},
			wantErr: "--hash is required",
		},
		{
			name:    "missing section",
			args:    []string{"--hash", "abcdef1234567"},
			wantErr: "--section is required",
		},
		{
			name:    "invalid hash uppercase",
			args:    []string{"--hash", "ABCDEF1234567", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid hash too short",
			args:    []string{"--hash", "abc12", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid hash non-hex",
			args:    []string{"--hash", "ghijklm1234567", "--section", "mig"},
			wantErr: "ca entry",
		},
		{
			name:    "invalid section unknown",
			args:    []string{"--hash", "abcdef1234567", "--section", "unknown"},
			wantErr: "invalid config ca section",
		},
		{
			name:    "invalid section server",
			args:    []string{"--hash", "abcdef1234567", "--section", "server"},
			wantErr: "invalid config ca section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCAUnset(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigCAUsage_ListsCurrentSectionsOnly(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	printConfigCAListUsage(buf)
	printConfigCASetUsage(buf)
	printConfigCAUnsetUsage(buf)
	out := buf.String()
	for _, token := range []string{"pre_gate", "post_gate", "mig"} {
		if !strings.Contains(out, token) {
			t.Fatalf("usage output missing %q: %s", token, out)
		}
	}
	for _, token := range []string{`\\bre_gate\\b`, `\\bheal\\b`} {
		if regexp.MustCompile(token).FindStringIndex(out) != nil {
			t.Fatalf("usage output unexpectedly contains %q: %s", token, out)
		}
	}
	for _, token := range []string{"sbom", "hook"} {
		if strings.Contains(out, token) {
			t.Fatalf("usage output unexpectedly contains %q: %s", token, out)
		}
	}
}
