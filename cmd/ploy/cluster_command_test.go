package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleClusterRequiresSubcommand verifies that handleCluster returns an error
// when no subcommand is provided, and prints the cluster usage information.
func TestHandleClusterRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster(nil, buf)

	// Should return an error indicating subcommand is required.
	if err == nil || !strings.Contains(err.Error(), "cluster subcommand required") {
		t.Fatalf("expected cluster subcommand error, got %v", err)
	}

	// Should print usage information.
	output := buf.String()
	if !strings.Contains(output, "Usage: ploy cluster") {
		t.Fatalf("expected cluster usage, got %q", output)
	}
}

// TestHandleClusterUnknownSubcommand verifies that handleCluster returns an error
// for unknown subcommands and prints the cluster usage.
func TestHandleClusterUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"unknown-cmd"}, buf)

	// Should return an error for unknown subcommand.
	if err == nil || !strings.Contains(err.Error(), "unknown cluster subcommand") {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}

	// Should print usage information.
	output := buf.String()
	if !strings.Contains(output, "Usage: ploy cluster") {
		t.Fatalf("expected cluster usage, got %q", output)
	}
}

// TestHandleClusterHelp verifies that handleCluster responds to --help and -h flags
// by printing usage and returning nil (no error).
func TestHandleClusterHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "--help", args: []string{"--help"}},
		{name: "-h", args: []string{"-h"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleCluster(tt.args, buf)

			// Should not return an error.
			if err != nil {
				t.Fatalf("expected no error for %s, got %v", tt.name, err)
			}

			// Should print usage information.
			output := buf.String()
			if !strings.Contains(output, "Usage: ploy cluster") {
				t.Fatalf("expected cluster usage for %s, got %q", tt.name, output)
			}

			// Should list all subcommands.
			expectedSubcommands := []string{"deploy", "node", "token"}
			for _, sub := range expectedSubcommands {
				if !strings.Contains(output, sub) {
					t.Errorf("expected usage to contain %q for %s, got %q", sub, tt.name, output)
				}
			}
		})
	}
}

// TestHandleClusterDelegatesNodeToHandleNode verifies that "cluster node"
// routes to handleNode. We test this by checking that no subcommand produces
// the node subcommand required error.
func TestHandleClusterDelegatesNodeToHandleNode(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"node"}, buf)

	// handleNode with no args requires a subcommand.
	if err == nil || !strings.Contains(err.Error(), "node subcommand required") {
		t.Fatalf("expected 'node subcommand required' error from handleNode, got %v", err)
	}
}

// TestHandleClusterDelegatesTokenToHandleToken verifies that "cluster token"
// routes to handleToken. We test this by checking that no subcommand produces
// the token subcommand required error.
func TestHandleClusterDelegatesTokenToHandleToken(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"token"}, buf)

	// handleToken with no args requires a subcommand.
	if err == nil || !strings.Contains(err.Error(), "token subcommand required") {
		t.Fatalf("expected 'token subcommand required' error from handleToken, got %v", err)
	}
}

// TestClusterNodeHelp verifies that "cluster node --help" works correctly.
func TestClusterNodeHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"node", "--help"}, buf)

	// Should not return an error.
	if err != nil {
		t.Fatalf("expected no error for cluster node --help, got %v", err)
	}

	// Should print node usage.
	output := buf.String()
	if !strings.Contains(output, "Usage: ploy cluster node") {
		t.Fatalf("expected node usage, got %q", output)
	}
}

// TestClusterTokenHelp verifies that "cluster token --help" works correctly.
// NOTE: Token operations are now accessible only via `ploy cluster token`.
func TestClusterTokenHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"token", "--help"}, buf)

	// Should not return an error.
	if err != nil {
		t.Fatalf("expected no error for cluster token --help, got %v", err)
	}

	// Should print token usage with cluster prefix.
	output := buf.String()
	if !strings.Contains(output, "Usage: ploy cluster token") {
		t.Fatalf("expected cluster token usage, got %q", output)
	}
}

// TestPrintClusterUsage verifies that printClusterUsage outputs the expected format.
func TestPrintClusterUsage(t *testing.T) {
	buf := &bytes.Buffer{}
	printClusterUsage(buf)

	output := buf.String()

	// Check for expected content.
	expectedStrings := []string{
		"Usage: ploy cluster <command>",
		"Commands:",
		"deploy",
		"node",
		"token",
		"Deploy runtime stack on the current host",
		"Manage worker nodes in a cluster",
		"Manage API tokens bound to a cluster",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

// TestClusterCommandIntegration tests the cluster command through the execute function
// to verify it's properly wired into the CLI.
func TestClusterCommandIntegration(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectContains []string
		expectNoError  bool
	}{
		{
			name:           "ploy cluster --help",
			args:           []string{"cluster", "--help"},
			expectContains: []string{"Usage: ploy cluster", "deploy", "node", "token"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster -h",
			args:           []string{"cluster", "-h"},
			expectContains: []string{"Usage: ploy cluster"},
			expectNoError:  true,
		},
		{
			name:           "ploy help cluster",
			args:           []string{"help", "cluster"},
			expectContains: []string{"Usage: ploy cluster"},
			expectNoError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd(tt.args, buf)

			if tt.expectNoError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			output := buf.String()
			for _, expected := range tt.expectContains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}
