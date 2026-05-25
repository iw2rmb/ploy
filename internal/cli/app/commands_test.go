package app

import (
	"bytes"
	"strings"
	"testing"
)

// TestCommandsWiredIntoRoot verifies that all command builders are properly
// wired into the root cobra command tree. This test ensures that the refactored
// command structure (using newMigCmd, newClusterCmd, etc.) properly integrates
// with the root command.
func TestCommandsWiredIntoRoot(t *testing.T) {
	// Create root command with stderr buffer to capture output.
	stderr := &bytes.Buffer{}
	rootCmd := NewRootCmd(stderr)

	// Verify that all expected commands are registered as subcommands.
	// The command names should match the "Use" field from each builder function.
	// NOTE: "token" has been removed from the top-level commands.
	// Token operations are now accessible only via `ploy cluster token`.
	expectedCommands := []string{
		"version",  // Built-in version command
		"mig",      // newMigCmd
		"run",      // newRunCmd
		"job",      // newJobCmd
		"pull",     // newPullCmd
		"cluster",  // newClusterCmd (includes token, node)
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

// TestRootCmdPreservesExistingBehavior verifies that the refactored root command
// preserves backward-compatible behavior for common use cases.
func TestRootCmdPreservesExistingBehavior(t *testing.T) {
	t.Run("version flag", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		rootCmd := NewRootCmdWithIO(stdout, stderr)
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
		rootCmd := NewRootCmd(stderr)
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

func TestCompletionCommandGeneratesScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "bash completion"},
		{shell: "zsh", want: "#compdef"},
		{shell: "fish", want: "complete -c ploy"},
		{shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.shell, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			rootCmd := NewRootCmdWithIO(stdout, stderr)
			rootCmd.SetArgs([]string{"completion", tt.shell})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("completion %s failed: %v", tt.shell, err)
			}
			if got := stdout.String(); !strings.Contains(got, tt.want) {
				t.Fatalf("completion %s output missing %q:\n%s", tt.shell, tt.want, got)
			}
		})
	}
}
