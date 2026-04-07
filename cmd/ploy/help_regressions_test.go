package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpRegressionLeafHelpReturnsNoError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args          []string
		expectSnippet string
	}{
		{args: []string{"run", "ls", "--help"}, expectSnippet: "Usage: ploy run ls"},
		{args: []string{"run", "cancel", "--help"}, expectSnippet: "Usage: ploy run cancel"},
		{args: []string{"run", "start", "--help"}, expectSnippet: "Usage: ploy run start"},
		{args: []string{"cluster", "node", "add", "--help"}, expectSnippet: "Usage: ploy cluster node add"},
		{args: []string{"cluster", "token", "list", "--help"}, expectSnippet: "Usage: ploy cluster token list"},
		{args: []string{"config", "gitlab", "show", "--help"}, expectSnippet: "Usage: ploy config gitlab show"},
		{args: []string{"config", "env", "set", "--help"}, expectSnippet: "Usage: ploy config env set"},
		{args: []string{"manifest", "schema", "--help"}, expectSnippet: "Usage: ploy manifest schema"},
		{args: []string{"manifest", "validate", "--help"}, expectSnippet: "Usage: ploy manifest validate"},
		{args: []string{"mig", "run", "repo", "--help"}, expectSnippet: "Usage: ploy mig run repo"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if err := executeCmd(tt.args, buf); err != nil {
				t.Fatalf("expected no error, got: %v\noutput:\n%s", err, buf.String())
			}
			if !strings.Contains(buf.String(), tt.expectSnippet) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.expectSnippet, buf.String())
			}
		})
	}
}

func TestHelpRegressionCommandRoutesDeepPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args          []string
		expectSnippet string
	}{
		{args: []string{"help", "manifest"}, expectSnippet: "Usage: ploy manifest <command>"},
		{args: []string{"help", "manifest", "schema"}, expectSnippet: "Usage: ploy manifest schema"},
		{args: []string{"help", "cluster", "node"}, expectSnippet: "Usage: ploy cluster node <command>"},
		{args: []string{"help", "run", "status"}, expectSnippet: "Usage: ploy run status [--json] <run-id>"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if err := executeCmd(tt.args, buf); err != nil {
				t.Fatalf("expected no error, got: %v\noutput:\n%s", err, buf.String())
			}
			if !strings.Contains(buf.String(), tt.expectSnippet) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.expectSnippet, buf.String())
			}
		})
	}
}
