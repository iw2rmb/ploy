package main

import (
	"bytes"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// TestHandleClusterRequiresSubcommand verifies that handleCluster returns an error
// when no subcommand is provided, and prints the cluster usage information.
func TestHandleClusterRequiresSubcommand(t *testing.T) {
	out := clienv.RunExpectError(t, handleCluster, nil, "cluster subcommand required")
	assertx.Contains(t, out, "Usage: ploy cluster")
}

// TestHandleClusterUnknownSubcommand verifies that handleCluster returns an error
// for unknown subcommands and prints the cluster usage.
func TestHandleClusterUnknownSubcommand(t *testing.T) {
	out := clienv.RunExpectError(t, handleCluster, []string{"unknown-cmd"}, "unknown cluster subcommand")
	assertx.Contains(t, out, "Usage: ploy cluster")
}

// TestHandleClusterHelp verifies that handleCluster responds to --help and -h flags
// by printing usage and returning nil (no error).
func TestHandleClusterHelp(t *testing.T) {
	clienv.RunHelp(t, handleCluster, nil, "Usage: ploy cluster", "deploy", "node", "token")
}

// TestHandleClusterDelegatesNodeToHandleNode verifies that "cluster node" routes to handleNode.
func TestHandleClusterDelegatesNodeToHandleNode(t *testing.T) {
	clienv.RunExpectError(t, handleCluster, []string{"node"}, "node subcommand required")
}

// TestHandleClusterDelegatesTokenToHandleToken verifies that "cluster token" routes to handleToken.
func TestHandleClusterDelegatesTokenToHandleToken(t *testing.T) {
	clienv.RunExpectError(t, handleCluster, []string{"token"}, "token subcommand required")
}

// TestClusterNodeHelp verifies that "cluster node --help" works correctly.
func TestClusterNodeHelp(t *testing.T) {
	out := clienv.RunExpectOK(t, handleCluster, []string{"node", "--help"})
	assertx.Contains(t, out, "Usage: ploy cluster node")
}

// TestClusterTokenHelp verifies that "cluster token --help" works correctly.
func TestClusterTokenHelp(t *testing.T) {
	out := clienv.RunExpectOK(t, handleCluster, []string{"token", "--help"})
	assertx.Contains(t, out, "Usage: ploy cluster token")
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
		assertx.Contains(t, output, expected)
	}
}

// TestClusterCommandIntegration tests the cluster command through the execute function
// to verify it's properly wired into the CLI.
func TestClusterCommandIntegration(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectContains []string
	}{
		{name: "ploy cluster --help", args: []string{"cluster", "--help"}, expectContains: []string{"Usage: ploy cluster", "deploy", "node", "token"}},
		{name: "ploy cluster -h", args: []string{"cluster", "-h"}, expectContains: []string{"Usage: ploy cluster"}},
		{name: "ploy help cluster", args: []string{"help", "cluster"}, expectContains: []string{"Usage: ploy cluster"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := clienv.RunExpectOK(t, executeCmd, tt.args)
			for _, want := range tt.expectContains {
				assertx.Contains(t, out, want)
			}
		})
	}
}
