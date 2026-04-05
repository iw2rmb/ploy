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
	rootCmd := newRootCmdWithIO(os.Stdout, os.Stderr)

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
	_, _ = fmt.Fprintln(w, "  mig              Plan and run Migs workflows")
	_, _ = fmt.Fprintln(w, "  run              Inspect runs and stream events")
	_, _ = fmt.Fprintln(w, "  job              Inspect and follow job logs")
	_, _ = fmt.Fprintln(w, "  pull             Pull Migs diffs for current repo HEAD")
	_, _ = fmt.Fprintln(w, "  cluster          Manage clusters (deploy, nodes, tokens)")
	_, _ = fmt.Fprintln(w, "  config           Inspect or update cluster configuration")
	_, _ = fmt.Fprintln(w, "  manifest         Inspect and validate integration manifests")
	_, _ = fmt.Fprintln(w, "  tui              Interactive TUI for migrations, runs, and jobs")
	// NOTE: `ploy token` has been removed as a top-level command.
	// Token operations are now accessible only via `ploy cluster token`.
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use 'ploy help <command>' for detailed command help.")
}
