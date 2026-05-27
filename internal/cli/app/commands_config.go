package app

import (
	"fmt"
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
		Use:   "config",
		Short: "Inspect or update cluster configuration",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	envCmd := &cobra.Command{
		Use:   "env",
		Short: "Manage global environment variables",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	envCmd.AddCommand(newConfigEnvListCmd(stderr))
	envCmd.AddCommand(newConfigEnvShowCmd(stderr))
	envCmd.AddCommand(newConfigEnvSetCmd(stderr))
	envCmd.AddCommand(newConfigEnvUnsetCmd(stderr))
	configCmd.AddCommand(envCmd)
	return configCmd
}

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
	return specCmd
}

// newClusterCmd creates the cobra command for 'ploy cluster' and its subcommands.
// This wires the cluster router into a proper cobra command hierarchy.
// The cluster command provides a unified namespace for cluster management:
//   - node:    Manage worker nodes in a cluster
//   - token:   Manage API tokens bound to a cluster
func newClusterCmd(stderr io.Writer) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage clusters (nodes, tokens)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	clusterCmd.AddCommand(newClusterNodeCmd(stderr))
	clusterCmd.AddCommand(newClusterTokenCmd(stderr))
	return clusterCmd
}

func newConfigEnvListCmd(stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List global environment variables",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configure.Handle([]string{"env", "list"}, stderr)
		},
	}
}

func newConfigEnvShowCmd(stderr io.Writer) *cobra.Command {
	var key, from string
	var raw bool
	cmd := &cobra.Command{
		Use:   "show --key <NAME>",
		Short: "Show a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"env", "show"}
			runArgs = addChangedString(cmd, runArgs, "key", key)
			runArgs = addChangedString(cmd, runArgs, "from", from)
			runArgs = addChangedBool(cmd, runArgs, "raw", raw)
			return configure.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&from, "from", "", "Target to read from")
	cmd.Flags().BoolVar(&raw, "raw", false, "Show raw value without redaction")
	return cmd
}

func newConfigEnvSetCmd(stderr io.Writer) *cobra.Command {
	var key, value, file string
	var on []string
	var secret bool
	cmd := &cobra.Command{
		Use:   "set --key <NAME> (--value <STRING> | --file <PATH>)",
		Short: "Set a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"env", "set"}
			runArgs = addChangedString(cmd, runArgs, "key", key)
			runArgs = addChangedString(cmd, runArgs, "value", value)
			runArgs = addChangedString(cmd, runArgs, "file", file)
			runArgs = addChangedStringArray(cmd, runArgs, "on", on)
			runArgs = addChangedBool(cmd, runArgs, "secret", secret)
			return configure.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&value, "value", "", "Inline value")
	cmd.Flags().StringVar(&file, "file", "", "Path to file containing value")
	cmd.Flags().StringArrayVar(&on, "on", nil, "Target selector: all, jobs, server, nodes, gates, steps")
	cmd.Flags().BoolVar(&secret, "secret", true, "Mark value as secret")
	return cmd
}

func newConfigEnvUnsetCmd(stderr io.Writer) *cobra.Command {
	var key, from string
	cmd := &cobra.Command{
		Use:   "unset --key <NAME>",
		Short: "Unset a global environment variable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"env", "unset"}
			runArgs = addChangedString(cmd, runArgs, "key", key)
			runArgs = addChangedString(cmd, runArgs, "from", from)
			return configure.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "Environment variable name")
	cmd.Flags().StringVar(&from, "from", "", "Target to delete from")
	return cmd
}

func newClusterNodeCmd(stderr io.Writer) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "Manage worker nodes in a cluster",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	nodeCmd.AddCommand(newClusterNodeAddCmd(stderr))
	nodeCmd.AddCommand(newClusterNodeActionsCmd(stderr))
	return nodeCmd
}

func newClusterNodeActionsCmd(stderr io.Writer) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "actions [--limit <n>] <node-id>",
		Short: "List recent worker node maintenance actions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"node", "actions"}
			runArgs = addChangedInt(cmd, runArgs, "limit", limit)
			runArgs = append(runArgs, args...)
			return cluster.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum actions to show")
	return cmd
}

func newClusterNodeAddCmd(stderr io.Writer) *cobra.Command {
	var clusterID, address, serverURL, identity, user, ploydNodeBinary string
	var sshPort int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add --cluster-id <id> --address <ip> --server-url <url>",
		Short: "Add a worker node to the cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"node", "add"}
			runArgs = addChangedString(cmd, runArgs, "cluster-id", clusterID)
			runArgs = addChangedString(cmd, runArgs, "address", address)
			runArgs = addChangedString(cmd, runArgs, "server-url", serverURL)
			runArgs = addChangedString(cmd, runArgs, "identity", identity)
			runArgs = addChangedString(cmd, runArgs, "user", user)
			runArgs = addChangedString(cmd, runArgs, "ployd-node-binary", ploydNodeBinary)
			runArgs = addChangedInt(cmd, runArgs, "ssh-port", sshPort)
			runArgs = addChangedBool(cmd, runArgs, "dry-run", dryRun)
			return cluster.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster-id", "", "Cluster identifier to join")
	cmd.Flags().StringVar(&address, "address", "", "Node IP or hostname")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "Ploy server URL")
	cmd.Flags().StringVar(&identity, "identity", "", "SSH private key used for provisioning")
	cmd.Flags().StringVar(&user, "user", "", "SSH username used for provisioning")
	cmd.Flags().StringVar(&ploydNodeBinary, "ployd-node-binary", "", "Path to the ployd-node binary")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 0, "SSH port for node provisioning")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate inputs without performing provisioning")
	return cmd
}

func newClusterTokenCmd(stderr io.Writer) *cobra.Command {
	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API tokens bound to a cluster",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	tokenCmd.AddCommand(newClusterTokenCreateCmd(stderr))
	tokenCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all API tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle([]string{"token", "list"}, stderr)
		},
	})
	tokenCmd.AddCommand(&cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cluster.Handle(append([]string{"token", "revoke"}, args...), stderr)
		},
	})
	return tokenCmd
}

func newClusterTokenCreateCmd(stderr io.Writer) *cobra.Command {
	var role, description string
	var expires int
	cmd := &cobra.Command{
		Use:   "create --role <role>",
		Short: "Create a new API token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runArgs := []string{"token", "create"}
			runArgs = addChangedString(cmd, runArgs, "role", role)
			runArgs = addChangedString(cmd, runArgs, "description", description)
			if cmd.Flags().Changed("expires") {
				runArgs = append(runArgs, "--expires", fmt.Sprintf("%d", expires))
			}
			return cluster.Handle(runArgs, stderr)
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "Token role: cli-admin, control-plane, or worker")
	cmd.Flags().StringVar(&description, "description", "", "Human-readable description")
	cmd.Flags().IntVar(&expires, "expires", 365, "Expiration in days")
	return cmd
}
