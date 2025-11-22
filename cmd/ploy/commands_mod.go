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

// newModsCmd creates the cobra command for 'ploy mods' (observe Mods execution).
func newModsCmd(stderr io.Writer) *cobra.Command {
	modsCmd := &cobra.Command{
		Use:                "mods",
		Short:              "Observe Mods execution (logs, events)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleMods(args, stderr)
		},
	}
	return modsCmd
}

// newRunsCmd creates the cobra command for 'ploy runs' (inspect/follow runs).
func newRunsCmd(stderr io.Writer) *cobra.Command {
	runsCmd := &cobra.Command{
		Use:                "runs",
		Short:              "Inspect and follow individual runs",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRuns(args, stderr)
		},
	}
	return runsCmd
}
