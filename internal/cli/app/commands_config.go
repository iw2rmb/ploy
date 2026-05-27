package app

import (
	"io"

	"github.com/iw2rmb/ploy/internal/cli/cluster"
	"github.com/iw2rmb/ploy/internal/cli/configure"
	"github.com/iw2rmb/ploy/internal/cli/spec"
	"github.com/spf13/cobra"
)

// newConfigCmd creates the cobra command tree for 'ploy config' and its subcommands.
// This wires existing config handlers into a proper cobra command hierarchy.
func newConfigCmd(stderr io.Writer) *cobra.Command {
	configCmd := &cobra.Command{
		Use:                "config",
		Short:              "Inspect or update cluster configuration",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configure.Handle(args, stderr)
		},
	}
	return configCmd
}

// newSpecCmd creates the cobra command tree for 'ploy spec' and its subcommands.
func newSpecCmd(stdout, stderr io.Writer) *cobra.Command {
	specCmd := &cobra.Command{
		Use:                "spec",
		Short:              "Inspect and validate mig specs",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Handle(args, stdout, stderr)
		},
	}
	return specCmd
}

// newClusterCmd creates the cobra command for 'ploy cluster' and its subcommands.
// This wires the cluster router into a proper cobra command hierarchy.
// The cluster command provides a unified namespace for cluster management:
//   - node:    Manage worker nodes in a cluster
//   - token:   Manage API tokens bound to a cluster
func newClusterCmd(stderr io.Writer) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:                "cluster",
		Short:              "Manage clusters (nodes, tokens)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle(args, stderr)
		},
	}
	return clusterCmd
}
