package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// TestHelpFlagsAtAllLevels verifies that --help and -h flags work at every command level,
// printing the correct usage and subcommand lists.
// NOTE: `ploy token` and `ploy server` were removed as top-level commands; token is under
// `ploy cluster token`.
func TestHelpFlagsAtAllLevels(t *testing.T) {
	tests := []struct {
		name           string
		baseArgs       []string
		expectContains []string
	}{
		{name: "ploy", baseArgs: nil, expectContains: []string{"Ploy CLI v2", "Core Commands:", "mig", "cluster"}},
		{name: "ploy mig", baseArgs: []string{"mig"}, expectContains: []string{"Usage: ploy mig", "run"}},
		{name: "ploy run", baseArgs: []string{"run"}, expectContains: []string{"Usage: ploy run"}},
		{name: "ploy config", baseArgs: []string{"config"}, expectContains: []string{"Usage: ploy config", "env"}},
		{name: "ploy spec", baseArgs: []string{"spec"}, expectContains: []string{"Usage: ploy spec", "schema", "validate"}},
		{name: "ploy cluster", baseArgs: []string{"cluster"}, expectContains: []string{"Usage: ploy cluster", "node", "token"}},
		{name: "ploy cluster node", baseArgs: []string{"cluster", "node"}, expectContains: []string{"Usage: ploy cluster node", "add", "actions"}},
		{name: "ploy cluster token", baseArgs: []string{"cluster", "token"}, expectContains: []string{"Usage: ploy cluster token", "create", "list", "revoke"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clienv.RunHelp(t, executeCmd, tt.baseArgs, tt.expectContains...)
		})
	}
}

// TestWantsHelpFunction tests the common.WantsHelp helper function.
func TestWantsHelpFunction(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{name: "single --help", args: []string{"--help"}, expected: true},
		{name: "single -h", args: []string{"-h"}, expected: true},
		{name: "empty args", args: []string{}, expected: false},
		{name: "nil args", args: nil, expected: false},
		{name: "subcommand only", args: []string{"deploy"}, expected: false},
		{name: "--help with extra arg", args: []string{"--help", "extra"}, expected: false},
		{name: "-h with extra arg", args: []string{"-h", "extra"}, expected: false},
		{name: "subcommand then --help", args: []string{"deploy", "--help"}, expected: false},
		{name: "--Help (wrong case)", args: []string{"--Help"}, expected: false},
		{name: "-H (wrong case)", args: []string{"-H"}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := common.WantsHelp(tt.args); got != tt.expected {
				t.Errorf("common.WantsHelp(%v) = %v, expected %v", tt.args, got, tt.expected)
			}
		})
	}
}

// TestHelpFlagNoUnknownSubcommandError verifies that --help does not trigger
// "unknown subcommand" errors that would be confusing to users.
func TestHelpFlagNoUnknownSubcommandError(t *testing.T) {
	commands := [][]string{
		{"mig", "--help"},
		{"config", "--help"},
		{"spec", "--help"},
		{"cluster", "--help"},
		{"cluster", "node", "--help"},
		{"cluster", "token", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out := clienv.RunExpectOK(t, executeCmd, args)
			lower := strings.ToLower(out)
			if strings.Contains(lower, "unknown") && strings.Contains(lower, "subcommand") {
				t.Errorf("output should not contain 'unknown subcommand' for help flag, got:\n%s", out)
			}
		})
	}
}

// TestRootHelpConsistency verifies that ploy --help and ploy help produce identical output.
func TestRootHelpConsistency(t *testing.T) {
	helpFlagBuf := &bytes.Buffer{}
	if err := executeCmd([]string{"--help"}, helpFlagBuf); err != nil {
		t.Fatalf("ploy --help failed: %v", err)
	}
	helpCmdBuf := &bytes.Buffer{}
	if err := executeCmd([]string{"help"}, helpCmdBuf); err != nil {
		t.Fatalf("ploy help failed: %v", err)
	}
	assertx.Contains(t, helpFlagBuf.String(), "Ploy CLI v2")
	if helpFlagBuf.String() != helpCmdBuf.String() {
		t.Errorf("ploy --help and ploy help produce different output:\n--help:\n%s\nhelp:\n%s",
			helpFlagBuf.String(), helpCmdBuf.String())
	}
}
