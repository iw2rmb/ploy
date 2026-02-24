package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHelpFlagsAtAllLevels verifies that --help and -h flags work correctly
// at every command level, printing the correct usage and subcommand lists
// instead of falling back to Cobra's default or surfacing "unknown subcommand" errors.
func TestHelpFlagsAtAllLevels(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectContains []string // Strings that must be present in output
		expectNoError  bool     // Whether the command should succeed (return nil)
	}{
		// Root level --help
		// NOTE: `ploy rollout` and `ploy token` have been removed as top-level commands.
		// Rollout is now accessible only via `ploy cluster rollout`.
		// Token is now accessible only via `ploy cluster token`.
		{
			name:           "ploy --help",
			args:           []string{"--help"},
			expectContains: []string{"Ploy CLI v2", "Core Commands:", "mig", "cluster"},
			expectNoError:  true,
		},
		{
			name:           "ploy -h",
			args:           []string{"-h"},
			expectContains: []string{"Ploy CLI v2", "Core Commands:"},
			expectNoError:  true,
		},

		// mig command --help
		{
			name:           "ploy mig --help",
			args:           []string{"mig", "--help"},
			expectContains: []string{"Usage: ploy mig", "run"},
			expectNoError:  true,
		},
		{
			name:           "ploy mig -h",
			args:           []string{"mig", "-h"},
			expectContains: []string{"Usage: ploy mig"},
			expectNoError:  true,
		},

		// run command --help
		{
			name:           "ploy run --help",
			args:           []string{"run", "--help"},
			expectContains: []string{"Usage: ploy run"},
			expectNoError:  true,
		},
		{
			name:           "ploy run -h",
			args:           []string{"run", "-h"},
			expectContains: []string{"Usage: ploy run"},
			expectNoError:  true,
		},

		// NOTE: `ploy server` has been removed as a top-level command.
		// Server deployment is now accessible only via `ploy cluster deploy`.
		// The server tests have been replaced with cluster deploy tests below.

		// cluster deploy --help (replaces ploy server --help)
		{
			name:           "ploy cluster deploy --help",
			args:           []string{"cluster", "deploy", "--help"},
			expectContains: []string{"Usage: ploy cluster deploy"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster deploy -h",
			args:           []string{"cluster", "deploy", "-h"},
			expectContains: []string{"Usage: ploy cluster deploy"},
			expectNoError:  true,
		},

		// NOTE: `ploy rollout` has been removed as a top-level command.
		// Rollout operations are now accessible only via `ploy cluster rollout`.
		// The rollout tests have been replaced with cluster rollout tests below.

		// config command --help
		{
			name:           "ploy config --help",
			args:           []string{"config", "--help"},
			expectContains: []string{"Usage: ploy config", "gitlab"},
			expectNoError:  true,
		},
		{
			name:           "ploy config -h",
			args:           []string{"config", "-h"},
			expectContains: []string{"Usage: ploy config"},
			expectNoError:  true,
		},

		// config gitlab --help (deeper level)
		{
			name:           "ploy config gitlab --help",
			args:           []string{"config", "gitlab", "--help"},
			expectContains: []string{"Usage: ploy config gitlab", "show", "set", "validate"},
			expectNoError:  true,
		},
		{
			name:           "ploy config gitlab -h",
			args:           []string{"config", "gitlab", "-h"},
			expectContains: []string{"Usage: ploy config gitlab"},
			expectNoError:  true,
		},

		// manifest command --help
		{
			name:           "ploy manifest --help",
			args:           []string{"manifest", "--help"},
			expectContains: []string{"Usage: ploy manifest", "schema", "validate"},
			expectNoError:  true,
		},
		{
			name:           "ploy manifest -h",
			args:           []string{"manifest", "-h"},
			expectContains: []string{"Usage: ploy manifest"},
			expectNoError:  true,
		},

		// NOTE: `ploy token` has been removed as a top-level command.
		// Token operations are now accessible only via `ploy cluster token`.
		// The ploy token --help tests have been removed from this section.
		// See cluster token --help tests below.

		// cluster command --help
		{
			name:           "ploy cluster --help",
			args:           []string{"cluster", "--help"},
			expectContains: []string{"Usage: ploy cluster", "deploy", "node", "rollout", "token"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster -h",
			args:           []string{"cluster", "-h"},
			expectContains: []string{"Usage: ploy cluster"},
			expectNoError:  true,
		},

		// cluster node --help (deeper level)
		{
			name:           "ploy cluster node --help",
			args:           []string{"cluster", "node", "--help"},
			expectContains: []string{"Usage: ploy cluster node", "add"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster node -h",
			args:           []string{"cluster", "node", "-h"},
			expectContains: []string{"Usage: ploy cluster node"},
			expectNoError:  true,
		},

		// cluster rollout --help (deeper level)
		// NOTE: Rollout is now accessible only via `ploy cluster rollout`.
		{
			name:           "ploy cluster rollout --help",
			args:           []string{"cluster", "rollout", "--help"},
			expectContains: []string{"Usage: ploy cluster rollout", "server", "nodes"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster rollout -h",
			args:           []string{"cluster", "rollout", "-h"},
			expectContains: []string{"Usage: ploy cluster rollout"},
			expectNoError:  true,
		},

		// cluster token --help (deeper level)
		// NOTE: Token operations are now accessible only via `ploy cluster token`.
		{
			name:           "ploy cluster token --help",
			args:           []string{"cluster", "token", "--help"},
			expectContains: []string{"Usage: ploy cluster token", "create", "list", "revoke"},
			expectNoError:  true,
		},
		{
			name:           "ploy cluster token -h",
			args:           []string{"cluster", "token", "-h"},
			expectContains: []string{"Usage: ploy cluster token"},
			expectNoError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd(tt.args, buf)

			// Check error expectation
			if tt.expectNoError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			// Check output contains expected strings
			output := buf.String()
			for _, expected := range tt.expectContains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

// TestWantsHelpFunction tests the wantsHelp helper function.
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
			result := wantsHelp(tt.args)
			if result != tt.expected {
				t.Errorf("wantsHelp(%v) = %v, expected %v", tt.args, result, tt.expected)
			}
		})
	}
}

// TestHelpFlagNoUnknownSubcommandError verifies that --help does not trigger
// "unknown subcommand" errors that would be confusing to users.
// NOTE: `ploy server`, `ploy rollout`, and `ploy token` have been removed as top-level commands.
// Server deployment is now only accessible via `ploy cluster deploy`.
// Rollout operations are now only accessible via `ploy cluster rollout`.
// Token operations are now only accessible via `ploy cluster token`.
func TestHelpFlagNoUnknownSubcommandError(t *testing.T) {
	commands := [][]string{
		{"mig", "--help"},
		// NOTE: {"server", "--help"} removed — server re-rooted under cluster deploy.
		// NOTE: {"rollout", "--help"} removed — rollout re-rooted under cluster rollout.
		// NOTE: {"token", "--help"} removed — token re-rooted under cluster token.
		{"config", "--help"},
		{"config", "gitlab", "--help"},
		{"manifest", "--help"},
		{"cluster", "--help"},
		{"cluster", "deploy", "--help"}, // Replaces ploy server --help
		{"cluster", "node", "--help"},
		{"cluster", "rollout", "--help"}, // Replaces ploy rollout --help
		{"cluster", "token", "--help"},   // Replaces ploy token --help
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := executeCmd(args, buf)

			// Should not return an error
			if err != nil {
				t.Errorf("expected no error for %v, got: %v", args, err)
			}

			// Output should NOT contain "unknown" or "subcommand" error messages
			output := buf.String()
			if strings.Contains(strings.ToLower(output), "unknown") && strings.Contains(strings.ToLower(output), "subcommand") {
				t.Errorf("output should not contain 'unknown subcommand' for help flag, got:\n%s", output)
			}
		})
	}
}

// TestRootHelpConsistency verifies that ploy --help and ploy help produce
// identical output, ensuring consistency in the CLI help system.
func TestRootHelpConsistency(t *testing.T) {
	// Get output from ploy --help
	helpFlagBuf := &bytes.Buffer{}
	errHelpFlag := executeCmd([]string{"--help"}, helpFlagBuf)

	// Get output from ploy help
	helpCmdBuf := &bytes.Buffer{}
	errHelpCmd := executeCmd([]string{"help"}, helpCmdBuf)

	// Both should succeed
	if errHelpFlag != nil {
		t.Errorf("ploy --help failed: %v", errHelpFlag)
	}
	if errHelpCmd != nil {
		t.Errorf("ploy help failed: %v", errHelpCmd)
	}

	// Output should be identical
	if helpFlagBuf.String() != helpCmdBuf.String() {
		t.Errorf("ploy --help and ploy help produce different output:\n--help:\n%s\nhelp:\n%s",
			helpFlagBuf.String(), helpCmdBuf.String())
	}
}
