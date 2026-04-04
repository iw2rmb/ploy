package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleConfigHome_SubcommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		wantOut string
	}{
		{
			name:    "requires subcommand",
			args:    nil,
			wantErr: "home subcommand required",
			wantOut: "Usage: ploy config home",
		},
		{
			name:    "unknown subcommand rejected",
			args:    []string{"unknown"},
			wantErr: "unknown home subcommand",
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
			err := handleConfigHome(tt.args, buf)
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

func TestHandleConfigHomeSet_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing entry",
			args:    []string{"--section", "mig"},
			wantErr: "--entry is required",
		},
		{
			name:    "missing section",
			args:    []string{"--entry", "abcdef1:.config/app"},
			wantErr: "--section is required",
		},
		{
			name:    "invalid entry missing dst",
			args:    []string{"--entry", "INVALID", "--section", "mig"},
			wantErr: "home entry",
		},
		{
			name:    "invalid entry absolute destination",
			args:    []string{"--entry", "abcdef1:/etc/passwd", "--section", "mig"},
			wantErr: "home entry",
		},
		{
			name:    "invalid entry path traversal",
			args:    []string{"--entry", "abcdef1:../escape", "--section", "mig"},
			wantErr: "home entry",
		},
		{
			name:    "invalid entry short hash",
			args:    []string{"--entry", "SHORT:.config/app", "--section", "mig"},
			wantErr: "home entry",
		},
		{
			name:    "invalid section unknown",
			args:    []string{"--entry", "abcdef1:.config/app", "--section", "unknown"},
			wantErr: "invalid hydra section",
		},
		{
			name:    "invalid section server",
			args:    []string{"--entry", "abcdef1:.config/app", "--section", "server"},
			wantErr: "invalid hydra section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigHomeSet(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHandleConfigHomeUnset_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing dst",
			args:    []string{"--section", "mig"},
			wantErr: "--dst is required",
		},
		{
			name:    "missing section",
			args:    []string{"--dst", ".config/app"},
			wantErr: "--section is required",
		},
		{
			name:    "invalid dst absolute path",
			args:    []string{"--dst", "/etc/passwd", "--section", "mig"},
			wantErr: "home destination",
		},
		{
			name:    "invalid dst path traversal",
			args:    []string{"--dst", "../escape", "--section", "mig"},
			wantErr: "home destination",
		},
		{
			name:    "invalid section unknown",
			args:    []string{"--dst", ".config/app", "--section", "unknown"},
			wantErr: "invalid hydra section",
		},
		{
			name:    "invalid section server",
			args:    []string{"--dst", ".config/app", "--section", "server"},
			wantErr: "invalid hydra section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigHomeUnset(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
