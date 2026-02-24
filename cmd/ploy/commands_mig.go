package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newMigCmd creates the cobra command tree for 'ploy mig' and its subcommands.
// This wires existing mig handlers into a proper cobra command hierarchy.
func newMigCmd(stderr io.Writer) *cobra.Command {
	// Top-level mig command — requires a subcommand.
	// When invoked without subcommand, the handler will print usage.
	migCmd := &cobra.Command{
		Use:                "mig",
		Short:              "Plan and run Migs workflows",
		DisableFlagParsing: true, // Handler does its own flag parsing for now
		RunE: func(cmd *cobra.Command, args []string) error {
			// Delegate to existing handler which manages subcommand dispatch.
			return handleMig(args, stderr)
		},
	}

	// Note: Subcommands are not yet explicitly defined here because handleMig
	// does manual dispatch. Future iterations will move subcommands into proper
	// cobra hierarchy with explicit AddCommand calls for run, fetch, cancel, etc.
	// For now, we preserve existing behavior by delegating to handleMig.

	return migCmd
}

// newRunCmd creates the cobra command for 'ploy run' (inspect/follow runs).
func newRunCmd(stderr io.Writer) *cobra.Command {
	runCmd := &cobra.Command{
		Use:                "run",
		Short:              "Inspect runs and stream events",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRun(args, stderr)
		},
	}
	return runCmd
}
