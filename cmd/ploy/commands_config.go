package main

import (
	"fmt"
	"io"

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
			return handleConfig(args, stderr)
		},
	}
	return configCmd
}

// newManifestCmd creates the cobra command tree for 'ploy manifest' and its subcommands.
func newManifestCmd(stderr io.Writer) *cobra.Command {
	manifestCmd := &cobra.Command{
		Use:                "manifest",
		Short:              "Inspect and validate integration manifests",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleManifest(args, stderr)
		},
	}
	return manifestCmd
}

// newTokenCmd creates the cobra command tree for 'ploy token' and its subcommands.
func newTokenCmd(stderr io.Writer) *cobra.Command {
	tokenCmd := &cobra.Command{
		Use:                "token",
		Short:              "Manage API tokens for authentication",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleToken(args, stderr)
		},
	}
	return tokenCmd
}

// newUploadCmd creates the cobra command for 'ploy upload'.
func newUploadCmd(stderr io.Writer) *cobra.Command {
	uploadCmd := &cobra.Command{
		Use:                "upload",
		Short:              "Upload artifact bundle to a run (HTTPS)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpload(args, stderr)
		},
	}
	return uploadCmd
}

// newClusterCmd creates the cobra command for 'ploy cluster'.
// This command is not yet implemented.
func newClusterCmd(stderr io.Writer) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:                "cluster",
		Short:              "Manage local cluster descriptors",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("cluster command not yet implemented")
		},
	}
	return clusterCmd
}
