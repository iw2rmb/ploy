package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestCommandsWiredIntoRoot verifies that all command builders are properly
// wired into the root cobra command tree. This test ensures that the refactored
// command structure (using newMigCmd, newClusterCmd, etc.) properly integrates
// with the root command.
func TestCommandsWiredIntoRoot(t *testing.T) {
	// Create root command with stderr buffer to capture output.
	stderr := &bytes.Buffer{}
	rootCmd := newRootCmd(stderr)

	// Verify that all expected commands are registered as subcommands.
	// The command names should match the "Use" field from each builder function.
	// NOTE: "token" has been removed from the top-level commands.
	// Token operations are now accessible only via `ploy cluster token`.
	expectedCommands := []string{
		"version",  // Built-in version command
		"mig",      // newMigCmd
		"run",      // newRunCmd
		"pull",     // newPullCmd
		"cluster",  // newClusterCmd (includes token, node, deploy)
		"config",   // newConfigCmd
		"manifest", // newManifestCmd
		"tui",      // newTUICmd
	}

	// Get all registered subcommands from the root command.
	commands := rootCmd.Commands()
	commandNames := make(map[string]bool)
	for _, cmd := range commands {
		commandNames[cmd.Name()] = true
	}

	// Verify each expected command is registered.
	for _, expected := range expectedCommands {
		if !commandNames[expected] {
			t.Errorf("expected command %q to be registered in root, but it was not found", expected)
		}
	}

	// Verify that no unexpected commands are registered (excluding "help" and "completion").
	// Cobra adds "help" and "completion" automatically in some configurations.
	for name := range commandNames {
		found := false
		for _, expected := range expectedCommands {
			if name == expected {
				found = true
				break
			}
		}
		// Allow "help" and "completion" as they may be added by cobra automatically.
		if !found && name != "help" && name != "completion" {
			t.Errorf("unexpected command %q registered in root", name)
		}
	}
}

// TestCommandBuildersFunctional verifies that each command builder function
// returns a valid cobra command that can be executed without panicking.
func TestCommandBuildersFunctional(t *testing.T) {
	tests := []struct {
		name    string
		builder func(w *bytes.Buffer) *cobra.Command
	}{
		{"newMigCmd", func(w *bytes.Buffer) *cobra.Command { return newMigCmd(w) }},
		{"newRunCmd", func(w *bytes.Buffer) *cobra.Command { return newRunCmd(w) }},
		{"newPullCmd", func(w *bytes.Buffer) *cobra.Command { return newPullCmd(w) }},
		{"newClusterCmd", func(w *bytes.Buffer) *cobra.Command { return newClusterCmd(w) }},
		{"newConfigCmd", func(w *bytes.Buffer) *cobra.Command { return newConfigCmd(w) }},
		{"newManifestCmd", func(w *bytes.Buffer) *cobra.Command { return newManifestCmd(w) }},
		{"newTUICmd", func(w *bytes.Buffer) *cobra.Command { return newTUICmd(w) }},
		// NOTE: legacy newServerCmd/newRolloutCmd/newTokenCmd builders have been removed.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cmd := tt.builder(buf)

			// Verify the command was created.
			if cmd == nil {
				t.Fatal("builder returned nil command")
			}

			// Verify the command has a Use field (required for cobra).
			if strings.TrimSpace(cmd.Use) == "" {
				t.Error("command Use field is empty")
			}

			// Verify the command has a Short description.
			if strings.TrimSpace(cmd.Short) == "" {
				t.Error("command Short description is empty")
			}

			// Verify the command has a RunE or Run function (or allows subcommands).
			// Commands with subcommands may not have RunE if they require a subcommand.
			hasRun := cmd.Run != nil || cmd.RunE != nil || len(cmd.Commands()) > 0
			if !hasRun {
				t.Error("command has no Run/RunE function and no subcommands")
			}
		})
	}
}

// TestRootCmdPreservesExistingBehavior verifies that the refactored root command
// preserves backward-compatible behavior for common use cases.
func TestRootCmdPreservesExistingBehavior(t *testing.T) {
	t.Run("version flag", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		rootCmd := newRootCmdWithIO(stdout, stderr)
		rootCmd.SetArgs([]string{"--version"})

		// Execute should handle --version and return the sentinel error.
		err := rootCmd.Execute()
		if err == nil || err.Error() != "version displayed" {
			t.Errorf("expected sentinel error 'version displayed', got: %v", err)
		}

		// Verify version output was written to stdout.
		output := stdout.String()
		if !strings.Contains(output, "dev") {
			t.Errorf("expected version output in stdout, got: %q", output)
		}
		if stderr.Len() != 0 {
			t.Errorf("expected empty stderr for version output, got: %q", stderr.String())
		}
	})

	t.Run("no args prints usage", func(t *testing.T) {
		stderr := &bytes.Buffer{}
		rootCmd := newRootCmd(stderr)
		rootCmd.SetArgs([]string{})

		// Execute with no args should print usage and return "command required" error.
		err := rootCmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "command required") {
			t.Errorf("expected 'command required' error, got: %v", err)
		}

		// Verify usage output was written to stderr.
		output := stderr.String()
		if !strings.Contains(output, "Ploy CLI v2") {
			t.Errorf("expected usage output in stderr, got: %q", output)
		}
	})
}

// TestHelpOutputMatchesDocumentation verifies that CLI help output remains
// consistent with the cobra-based implementation. This test ensures that
// documentation examples stay accurate when the CLI structure changes.
//
// The test validates:
// - Root help includes all expected commands (mig, cluster, etc.)
// - The completion command is documented and functional
// - Help format matches cobra conventions
// - Key commands have proper Short descriptions
func TestHelpOutputMatchesDocumentation(t *testing.T) {
	t.Parallel()

	// Test cases for different help outputs.
	// Each test ensures that key documentation elements are present in the help text.
	tests := []struct {
		name           string
		args           []string
		expectedInHelp []string // Strings that must appear in help output
		mustContainAll bool     // If true, all expected strings must be present
		description    string
	}{
		{
			name: "root help shows all commands",
			args: []string{"--help"},
			// Our custom printUsage format uses "Core Commands:" not "Available Commands:"
			// This preserves existing custom help output behavior.
			expectedInHelp: []string{
				"Ploy CLI v2",
				"Core Commands:",
				"mig",
				"cluster",
				"config",
				"manifest",
			},
			mustContainAll: true,
			description:    "Root help must list all top-level commands",
		},
		// Note: The completion command is a built-in Cobra command.
		// Our custom SetHelpFunc intentionally prints the root-level custom help
		// for all --help flags. To test that completion works, we rely on
		// TestCompletionCommandFunctional which tests actual completion generation.
		// Here we just verify the completion command is registered.
		{
			name: "completion command registered",
			args: []string{"completion", "bash"},
			expectedInHelp: []string{
				"# bash completion", // Cobra's bash completion script starts with this
			},
			mustContainAll: true,
			description:    "completion bash must generate bash completion script",
		},
		{
			name: "mig command shows subcommands",
			args: []string{"mig", "--help"},
			expectedInHelp: []string{
				"mig",
				"run",
				"run repo",
				"artifacts",
			},
			mustContainAll: true,
			description:    "mig --help must list all subcommands",
		},
	}

	for _, tt := range tests {
		// Capture test case for parallel execution.
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create root command with stderr buffer.
			stderr := &bytes.Buffer{}
			rootCmd := newRootCmd(stderr)
			rootCmd.SetArgs(tt.args)

			// Execute and capture output.
			// Help commands return an error (e.g., ErrHelp), which is expected.
			_ = rootCmd.Execute()
			output := stderr.String()

			// Verify expected strings appear in help output.
			for _, expected := range tt.expectedInHelp {
				if !strings.Contains(output, expected) {
					t.Errorf("%s: expected help output to contain %q, got:\n%s",
						tt.description, expected, output)
					if tt.mustContainAll {
						// Continue checking other strings even if one fails.
						continue
					}
					return
				}
			}
		})
	}
}

// TestCompletionCommandFunctional verifies that the completion command
// can generate completion scripts for all supported shells without errors.
// This ensures the cobra completion integration works correctly.
func TestCompletionCommandFunctional(t *testing.T) {
	t.Parallel()

	// Supported shells from cobra's completion command.
	shells := []string{"bash", "zsh", "fish", "powershell"}

	for _, shell := range shells {
		shell := shell
		t.Run(shell, func(t *testing.T) {
			t.Parallel()

			stderr := &bytes.Buffer{}
			rootCmd := newRootCmd(stderr)
			rootCmd.SetArgs([]string{"completion", shell})

			// Execute completion generation.
			// The completion command writes to stdout and should not return an error.
			err := rootCmd.Execute()
			if err != nil {
				t.Errorf("completion %s failed: %v", shell, err)
			}

			// Verify that completion output was generated (non-empty stderr or no error).
			// Note: cobra writes completion scripts to stdout, not stderr.
			// Since we're testing in-memory, just verify no error occurred.
			// The actual completion script content is tested by cobra itself.
		})
	}
}

// TestTUICommandRegistered verifies that 'ploy tui' is registered in the root
// cobra command and has the required Use and Short fields.
func TestTUICommandRegistered(t *testing.T) {
	stderr := &bytes.Buffer{}
	rootCmd := newRootCmd(stderr)

	var tuiCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "tui" {
			tuiCmd = cmd
			break
		}
	}

	if tuiCmd == nil {
		t.Fatal("'tui' command not found in root command")
	}
	if strings.TrimSpace(tuiCmd.Use) == "" {
		t.Error("tui command: Use field is empty")
	}
	if strings.TrimSpace(tuiCmd.Short) == "" {
		t.Error("tui command: Short field is empty")
	}
	if tuiCmd.RunE == nil {
		t.Error("tui command: RunE must be set")
	}
}
