package app

import (
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/cluster"
	"github.com/iw2rmb/ploy/internal/cli/spec"
	"github.com/spf13/cobra"
)

// newSpecCmd creates the cobra command tree for 'ploy spec' and its subcommands.
func newSpecCmd(stdout, stderr io.Writer) *cobra.Command {
	specCmd := &cobra.Command{
		Use:   "spec",
		Short: "Inspect and validate mig specs",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	specCmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "Print the mig JSON schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Handle([]string{"schema"}, stdout, stderr)
		},
	})
	specCmd.AddCommand(&cobra.Command{
		Use:   "validate <path> [<path>...]",
		Short: "Validate mig specs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Handle(append([]string{"validate"}, args...), stdout, stderr)
		},
	})
	listCmd := &cobra.Command{
		Use:   "ls",
		Short: "List named mig specs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Handle([]string{"ls"}, stdout, stderr)
		},
	}
	specCmd.AddCommand(listCmd)
	return specCmd
}

// newClusterCmd creates the cobra command for 'ploy cluster' and its subcommands.
// This wires the cluster router into a proper cobra command hierarchy.
// The cluster command provides a unified namespace for node and token operations.
func newClusterCmd(stdout, stderr io.Writer) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage nodes and API tokens",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	clusterCmd.AddCommand(newClusterNodeCmd(stdout, stderr))
	clusterCmd.AddCommand(newClusterTokenCmd(stdout, stderr))
	return clusterCmd
}

func newClusterNodeCmd(stdout, stderr io.Writer) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "Manage worker nodes",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	nodeCmd.AddCommand(newClusterNodeAddCmd(stdout, stderr))
	nodeCmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List worker nodes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle([]string{"node", "ls"}, stdout, stderr)
		},
	})
	nodeCmd.AddCommand(&cobra.Command{
		Use:   "inspect <name>",
		Short: "Inspect a worker node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle([]string{"node", "inspect", args[0]}, stdout, stderr)
		},
	})
	return nodeCmd
}

func newClusterNodeAddCmd(stdout, stderr io.Writer) *cobra.Command {
	var address, serverURL, identity, user, ploydNodeBinary string
	var sshPort int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add --address <ip> --server-url <url>",
		Short: "Add a worker node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"node", "add"}
			runArgs = addChangedString(cmd, runArgs, "address", address)
			runArgs = addChangedString(cmd, runArgs, "server-url", serverURL)
			runArgs = addChangedString(cmd, runArgs, "identity", identity)
			runArgs = addChangedString(cmd, runArgs, "user", user)
			runArgs = addChangedString(cmd, runArgs, "ployd-node-binary", ploydNodeBinary)
			runArgs = addChangedInt(cmd, runArgs, "ssh-port", sshPort)
			runArgs = addChangedBool(cmd, runArgs, "dry-run", dryRun)
			return cluster.Handle(runArgs, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Node IP or hostname")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "Ploy server URL")
	cmd.Flags().StringVar(&identity, "identity", "", "SSH private key used for provisioning")
	cmd.Flags().StringVar(&user, "user", "", "SSH username used for provisioning")
	cmd.Flags().StringVar(&ploydNodeBinary, "ployd-node-binary", "", "Path to the ployd-node binary")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 0, "SSH port for node provisioning")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate inputs without performing provisioning")
	return cmd
}

func newClusterTokenCmd(stdout, stderr io.Writer) *cobra.Command {
	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API tokens",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	tokenCmd.AddCommand(newClusterTokenCreateCmd(stdout, stderr))
	tokenCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all API tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle([]string{"token", "list"}, stdout, stderr)
		},
	})
	tokenCmd.AddCommand(&cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle(append([]string{"token", "revoke"}, args...), stdout, stderr)
		},
	})
	return tokenCmd
}

func newClusterTokenCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	var role, username, description string
	var expires int
	cmd := &cobra.Command{
		Use:   "create --role <role>",
		Short: "Create a new API token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"token", "create"}
			runArgs = addChangedString(cmd, runArgs, "role", role)
			runArgs = addChangedString(cmd, runArgs, "username", username)
			runArgs = addChangedString(cmd, runArgs, "description", description)
			if cmd.Flags().Changed("expires") {
				runArgs = append(runArgs, "--expires", fmt.Sprintf("%d", expires))
			}
			return cluster.Handle(runArgs, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "Token role: cli-admin, control-plane, or worker")
	cmd.Flags().StringVar(&username, "username", "", "Durable username for control-plane tokens")
	cmd.Flags().StringVar(&description, "description", "", "Human-readable description")
	cmd.Flags().IntVar(&expires, "expires", 365, "Expiration in days")
	return cmd
}
