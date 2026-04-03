package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestConfigCommand_HydraSubcommandRouting verifies that the config command
// surface routes ca and home typed Hydra subcommands correctly.
func TestConfigCommand_HydraSubcommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "ca routes to handleConfigCA",
			args:    []string{"ca"},
			wantErr: "ca subcommand required",
		},
		{
			name:    "home routes to handleConfigHome",
			args:    []string{"home"},
			wantErr: "home subcommand required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfig(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestConfigCommand_HydraHomeSetValidation verifies that the home set command
// validates required flags before reaching the server.
func TestConfigCommand_HydraHomeSetValidation(t *testing.T) {
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
			name:    "empty entry",
			args:    []string{"--entry", "", "--section", "mig"},
			wantErr: "--entry is required",
		},
		{
			name:    "empty section",
			args:    []string{"--entry", "abcdef1:.config/app", "--section", ""},
			wantErr: "--section is required",
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
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestConfigCommand_HydraHomeUnsetValidation verifies that the home unset
// command validates required flags before reaching the server.
func TestConfigCommand_HydraHomeUnsetValidation(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigHomeUnset(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestConfigCommand_HydraCASetValidation verifies that the CA set command
// validates required flags before reaching the server.
func TestConfigCommand_HydraCASetValidation(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCASet(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestConfigCommand_HydraCAUnsetValidation verifies that the CA unset command
// validates required flags before reaching the server.
func TestConfigCommand_HydraCAUnsetValidation(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleConfigCAUnset(tt.args, buf)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
