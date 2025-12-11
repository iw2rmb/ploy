package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newModCmd creates the cobra command tree for 'ploy mod' and its subcommands.
// This wires existing mod handlers into a proper cobra command hierarchy.
func newModCmd(stderr io.Writer) *cobra.Command {
	// Top-level mod command — requires a subcommand.
	// When invoked without subcommand, the handler will print usage.
	modCmd := &cobra.Command{
		Use:                "mod",
		Short:              "Plan and run Mods workflows",
		DisableFlagParsing: true, // Handler does its own flag parsing for now
		RunE: func(cmd *cobra.Command, args []string) error {
			// Delegate to existing handler which manages subcommand dispatch.
			return handleMod(args, stderr)
		},
	}

	// Note: Subcommands are not yet explicitly defined here because handleMod
	// does manual dispatch. Future iterations will move subcommands into proper
	// cobra hierarchy with explicit AddCommand calls for run, fetch, cancel, etc.
	// For now, we preserve existing behavior by delegating to handleMod.

	return modCmd
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
