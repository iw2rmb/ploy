package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	iversion "github.com/iw2rmb/ploy/internal/version"
)

// newRootCmd constructs the cobra root command with all subcommands.
// It preserves the existing CLI surface and error reporting behavior.
func newRootCmd(stderr io.Writer) *cobra.Command {
	// Root command — top-level ploy entry point.
	// SilenceUsage prevents cobra from printing usage on every error.
	// SilenceErrors allows us to control error formatting via reportError.
	// RunE returns an error when called with no subcommand, matching old behavior.
	root := &cobra.Command{
		Use:           "ploy",
		Short:         "Ploy CLI v2",
		Long:          "Ploy CLI v2 — control plane and node management",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When root is invoked with no subcommand, print usage and return error.
			// This matches the old execute() behavior: printUsage + "command required" error.
			printUsage(stderr)
			return fmt.Errorf("command required")
		},
	}

	// Version command: prints version information.
	// Preserves the behavior of "ploy version", "ploy --version", "ploy -version".
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion(stderr)
		},
	}
	root.AddCommand(versionCmd)

	// Add top-level --version flag to match existing behavior.
	root.Flags().BoolP("version", "v", false, "Print version information")
	root.PreRunE = func(cmd *cobra.Command, args []string) error {
		// Handle --version or -v flag at root level.
		versionFlag, _ := cmd.Flags().GetBool("version")
		if versionFlag {
			printVersion(stderr)
			// Return a sentinel error to skip execution (cobra will not print it).
			return fmt.Errorf("version displayed")
		}
		return nil
	}

	// Subcommands: wire existing handlers into cobra commands.
	// Commands are structured via dedicated builder functions (newMigCmd, newClusterCmd, etc.)
	// that encapsulate command hierarchy and preserve existing business logic.
	// Each builder function creates a cobra command tree with proper subcommand structure.

	// Mods workflow commands
	root.AddCommand(newMigCmd(stderr))  // ploy mig (run, fetch, cancel, inspect, artifacts, diffs)
	root.AddCommand(newRunCmd(stderr))  // ploy run (events, inspect)
	root.AddCommand(newPullCmd(stderr)) // ploy pull (local repo pull workflow)

	// Cluster and configuration commands
	root.AddCommand(newClusterCmd(stderr))  // ploy cluster (deploy, node, token)
	root.AddCommand(newConfigCmd(stderr))   // ploy config (gitlab show/set/validate)
	root.AddCommand(newManifestCmd(stderr)) // ploy manifest (schema, validate)

	// Interactive TUI
	root.AddCommand(newTUICmd(stderr)) // ploy tui (interactive terminal UI)

	// Server, node, and token management commands
	// NOTE: `ploy server`, `ploy node`, and `ploy token` have been removed as top-level commands.
	// Runtime deployment is accessible via `ploy cluster deploy`.
	// Node operations are now accessible only via `ploy cluster node`.
	// Token operations are now accessible only via `ploy cluster token`.
	// This keeps deployment and node-management under `ploy cluster ...` and reduces top-level command surface.

	// Override help function so that `ploy --help` and `ploy -h` print our
	// custom usage output instead of Cobra's default help format.
	// This ensures consistency between `ploy --help` and `ploy help`.
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printUsage(stderr)
	})

	// Override help command to preserve existing behavior.
	// Cobra provides a default help command, but we want to preserve printUsage logic.
	// We replace the default help with a custom implementation.
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Run: func(cmd *cobra.Command, args []string) {
			printRequestedHelp(stderr, args)
		},
	})

	// Set output to stderr for all cobra messages.
	root.SetOut(stderr)
	root.SetErr(stderr)

	// Configure unknown command handling to match old behavior.
	// When an unknown subcommand is encountered, print usage and return a descriptive error.
	// Cobra's default ValidArgsFunction doesn't apply here; instead, we use a custom approach.
	// We set a flag validator that will be called when cobra encounters an unknown command.
	// However, cobra's built-in unknown command handling already returns an error like "unknown command X for ploy".
	// To also print usage (matching old behavior), we wrap the execution or use a custom error handler.
	// A simpler approach: configure FParseErrWhitelist or use cobra's built-in behavior with custom messages.
	// Let's use cobra's Args field to enforce validation if needed, but cobra already handles unknown commands well.
	// For now, we rely on cobra's default "unknown command" error, which is sufficient.
	// To match the old behavior of printing usage on unknown command, we could use a custom SuggestionsMinimumDistance
	// or intercept in the RunE. But the test expects printUsage to be called.
	// Let's add a FlagErrorFunc to print usage on flag/command errors.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		// Print usage when a flag or command error occurs.
		printUsage(stderr)
		return err
	})

	return root
}

func printRequestedHelp(w io.Writer, args []string) {
	if len(args) == 0 {
		printUsage(w)
		return
	}

	withHelp := ensureHelpArg(args[1:])
	switch args[0] {
	case "mig":
		_ = handleMig(withHelp, w)
	case "run":
		_ = handleRun(withHelp, w)
	case "pull":
		_ = handlePull(withHelp, w)
	case "cluster":
		_ = handleCluster(withHelp, w)
	case "config":
		_ = handleConfig(withHelp, w)
	case "manifest":
		_ = handleManifest(withHelp, w)
	case "tui":
		printTUIUsage(w)
	case "version":
		_, _ = fmt.Fprintln(w, "Usage: ploy version")
	default:
		printUsage(w)
	}
}

func ensureHelpArg(args []string) []string {
	if len(args) == 0 {
		return []string{"--help"}
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return args
		}
	}
	return append(args, "--help")
}

// printVersion outputs version information to the given writer.
// Preserves the existing version format.
func printVersion(w io.Writer) {
	v := iversion.Version
	if strings.TrimSpace(v) == "" {
		v = "dev"
	}
	_, _ = fmt.Fprintf(w, "ploy version %s\n", v)
	if iversion.Commit != "" || iversion.BuiltAt != "" {
		_, _ = fmt.Fprintf(w, "commit %s\n", iversion.Commit)
		if iversion.BuiltAt != "" {
			_, _ = fmt.Fprintf(w, "built  %s\n", iversion.BuiltAt)
		}
	}
}
