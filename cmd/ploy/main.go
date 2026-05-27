package main

import (
	"fmt"
	"os"

	"github.com/iw2rmb/ploy/internal/cli/app"
)

// main bootstraps the CLI entrypoint using cobra.
// Cobra handles command routing, flag parsing, and help generation.
func main() {
	// Construct the root cobra command with all subcommands.
	rootCmd := app.NewRootCmdWithIO(os.Stdout, os.Stderr)

	// Execute the root command with os.Args (cobra handles os.Args internally).
	// Cobra's Execute() method processes os.Args[1:] automatically.
	if err := rootCmd.Execute(); err != nil {
		// Cobra's SilenceErrors is set, so we control error output here.
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
