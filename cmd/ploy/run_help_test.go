package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunStatusHelpUsesStatusUsage(t *testing.T) {
	t.Helper()

	tests := []struct {
		name string
		flag string
	}{
		{name: "long help flag", flag: "--help"},
		{name: "short help flag", flag: "-h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd([]string{"run", "status", tt.flag}, buf)
			if err != nil {
				t.Fatalf("run status %s: %v", tt.flag, err)
			}
			out := buf.String()
			if !strings.Contains(out, "Usage: ploy run status [--json] <run-id>") {
				t.Fatalf("expected status usage, got: %q", out)
			}
			if strings.Contains(out, "Usage: ploy run <command>") {
				t.Fatalf("expected no top-level run usage, got: %q", out)
			}
		})
	}
}

func TestRunLogsHelpUsesLogsUsage(t *testing.T) {
	t.Helper()

	tests := []struct {
		name string
		flag string
	}{
		{name: "long help flag", flag: "--help"},
		{name: "short help flag", flag: "-h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd([]string{"run", "logs", tt.flag}, buf)
			if err != nil {
				t.Fatalf("run logs %s: %v", tt.flag, err)
			}
			out := buf.String()
			if !strings.Contains(out, "Usage: ploy run logs [--format <raw|structured>] [--max-retries <n>] [--idle-timeout <duration>] [--timeout <duration>] <run-id>") {
				t.Fatalf("expected logs usage, got: %q", out)
			}
			if strings.Contains(out, "Usage: ploy run <command>") {
				t.Fatalf("expected no top-level run usage, got: %q", out)
			}
		})
	}
}
