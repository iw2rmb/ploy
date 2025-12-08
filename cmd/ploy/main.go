package main

import (
	"fmt"
	"io"
	"os"
)

// main bootstraps the CLI entrypoint using cobra.
// Cobra handles command routing, flag parsing, and help generation.
func main() {
	// Construct the root cobra command with all subcommands.
	rootCmd := newRootCmd(os.Stderr)

	// Execute the root command with os.Args (cobra handles os.Args internally).
	// Cobra's Execute() method processes os.Args[1:] automatically.
	if err := rootCmd.Execute(); err != nil {
		// Cobra's SilenceErrors is set, so we control error output here.
		// Skip reporting if it's the sentinel "version displayed" error.
		if err.Error() != "version displayed" {
			reportError(err, os.Stderr)
			os.Exit(1)
		}
		// For "version displayed", exit cleanly without error message.
		os.Exit(0)
	}
}

// reportError prints a standard error prefix for CLI failures.
// Preserves the existing error reporting format.
func reportError(err error, stderr io.Writer) {
	_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
}

// printUsage lists the available top-level commands.
// Preserves the existing usage format exactly.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Ploy CLI v2")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  ploy <command> [<args>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Core Commands:")
	_, _ = fmt.Fprintln(w, "  mod              Plan and run Mods workflows")
	_, _ = fmt.Fprintln(w, "  mods             Observe Mods execution (logs, events)")
	_, _ = fmt.Fprintln(w, "  runs             Inspect and follow individual runs")
	_, _ = fmt.Fprintln(w, "  upload           Upload artifact bundle to a run (HTTPS)")
	_, _ = fmt.Fprintln(w, "  cluster          Manage clusters (deploy, nodes, rollout, tokens)")
	_, _ = fmt.Fprintln(w, "  config           Inspect or update cluster configuration")
	_, _ = fmt.Fprintln(w, "  manifest         Inspect and validate integration manifests")
	_, _ = fmt.Fprintln(w, "  node             Manage worker nodes")
	_, _ = fmt.Fprintln(w, "  rollout          Rolling updates for servers and nodes")
	_, _ = fmt.Fprintln(w, "  token            Manage API tokens for authentication")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use 'ploy help <command>' for detailed command help.")
}

// execute provides backward compatibility for existing tests.
// It wraps the cobra root command execution with the old function signature.
// This allows existing tests to continue working without modification.
func execute(args []string, stderr io.Writer) error {
	rootCmd := newRootCmd(stderr)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
