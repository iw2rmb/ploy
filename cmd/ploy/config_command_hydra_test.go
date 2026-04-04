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

