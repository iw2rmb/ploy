package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestCommandsWiredIntoRoot verifies that all command builders are properly
// wired into the root cobra command tree. This test ensures that the refactored
// command structure (using newModCmd, newServerCmd, etc.) properly integrates
// with the root command.
func TestCommandsWiredIntoRoot(t *testing.T) {
	// Create root command with stderr buffer to capture output.
	stderr := &bytes.Buffer{}
	rootCmd := newRootCmd(stderr)

	// Verify that all expected commands are registered as subcommands.
	// The command names should match the "Use" field from each builder function.
	expectedCommands := []string{
		"version",        // Built-in version command
		"mod",            // newModCmd
		"mods",           // newModsCmd
		"runs",           // newRunsCmd
		"upload",         // newUploadCmd
		"cluster",        // newClusterCmd
		"config",         // newConfigCmd
		"manifest",       // newManifestCmd
		"knowledge-base", // newKnowledgeBaseCmd
		"server",         // newServerCmd
		"node",           // newNodeCmd
		"rollout",        // newRolloutCmd
		"token",          // newTokenCmd
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
		{"newModCmd", func(w *bytes.Buffer) *cobra.Command { return newModCmd(w) }},
		{"newModsCmd", func(w *bytes.Buffer) *cobra.Command { return newModsCmd(w) }},
		{"newRunsCmd", func(w *bytes.Buffer) *cobra.Command { return newRunsCmd(w) }},
		{"newUploadCmd", func(w *bytes.Buffer) *cobra.Command { return newUploadCmd(w) }},
		{"newClusterCmd", func(w *bytes.Buffer) *cobra.Command { return newClusterCmd(w) }},
		{"newConfigCmd", func(w *bytes.Buffer) *cobra.Command { return newConfigCmd(w) }},
		{"newManifestCmd", func(w *bytes.Buffer) *cobra.Command { return newManifestCmd(w) }},
		{"newKnowledgeBaseCmd", func(w *bytes.Buffer) *cobra.Command { return newKnowledgeBaseCmd(w) }},
		{"newServerCmd", func(w *bytes.Buffer) *cobra.Command { return newServerCmd(w) }},
		{"newNodeCmd", func(w *bytes.Buffer) *cobra.Command { return newNodeCmd(w) }},
		{"newRolloutCmd", func(w *bytes.Buffer) *cobra.Command { return newRolloutCmd(w) }},
		{"newTokenCmd", func(w *bytes.Buffer) *cobra.Command { return newTokenCmd(w) }},
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
		stderr := &bytes.Buffer{}
		rootCmd := newRootCmd(stderr)
		rootCmd.SetArgs([]string{"--version"})

		// Execute should handle --version and return the sentinel error.
		err := rootCmd.Execute()
		if err == nil || err.Error() != "version displayed" {
			t.Errorf("expected sentinel error 'version displayed', got: %v", err)
		}

		// Verify version output was written to stderr.
		output := stderr.String()
		if !strings.Contains(output, "ploy version") {
			t.Errorf("expected version output in stderr, got: %q", output)
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
