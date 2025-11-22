package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newServerCmd creates the cobra command tree for 'ploy server' and its subcommands.
// This wires existing server handlers into a proper cobra command hierarchy.
func newServerCmd(stderr io.Writer) *cobra.Command {
	serverCmd := &cobra.Command{
		Use:                "server",
		Short:              "Manage control plane server",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleServer(args, stderr)
		},
	}
	return serverCmd
}

// newRolloutCmd creates the cobra command tree for 'ploy rollout' and its subcommands.
func newRolloutCmd(stderr io.Writer) *cobra.Command {
	rolloutCmd := &cobra.Command{
		Use:                "rollout",
		Short:              "Rolling updates for servers and nodes",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRollout(args, stderr)
		},
	}
	return rolloutCmd
}

// newNodeCmd creates the cobra command for 'ploy node'.
func newNodeCmd(stderr io.Writer) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:                "node",
		Short:              "Manage worker nodes",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleNode(args, stderr)
		},
	}
	return nodeCmd
}
