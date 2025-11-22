package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	iversion "github.com/iw2rmb/ploy/internal/version"
)

// newRootCmd constructs the cobra root command with all subcommands.
// It preserves the existing CLI surface and error reporting behavior.
func newRootCmd(stderr io.Writer) *cobra.Command {
	// Root command — top-level ploy entry point.
	// SilenceUsage prevents cobra from printing usage on every error.
	// SilenceErrors allows us to control error formatting via reportError.
	// RunE returns an error when called with no subcommand, matching old behavior.
	root := &cobra.Command{
		Use:           "ploy",
		Short:         "Ploy CLI v2",
		Long:          "Ploy CLI v2 — control plane and node management",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When root is invoked with no subcommand, print usage and return error.
			// This matches the old execute() behavior: printUsage + "command required" error.
			printUsage(stderr)
			return fmt.Errorf("command required")
		},
	}

	// Version command: prints version information.
	// Preserves the behavior of "ploy version", "ploy --version", "ploy -version".
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion(stderr)
		},
	}
	root.AddCommand(versionCmd)

	// Add top-level --version flag to match existing behavior.
	root.Flags().BoolP("version", "v", false, "Print version information")
	root.PreRunE = func(cmd *cobra.Command, args []string) error {
		// Handle --version or -v flag at root level.
		versionFlag, _ := cmd.Flags().GetBool("version")
		if versionFlag {
			printVersion(stderr)
			// Return a sentinel error to skip execution (cobra will not print it).
			return fmt.Errorf("version displayed")
		}
		return nil
	}

	// Subcommands: wire existing handlers into cobra commands.
	// Each command delegates to the existing handler functions, preserving business logic.

	// mod command and subcommands.
	// DisableFlagParsing allows the existing handler to parse flags itself.
	modCmd := &cobra.Command{
		Use:                "mod",
		Short:              "Plan and run Mods workflows",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleMod(args, stderr)
		},
	}
	root.AddCommand(modCmd)

	// mods command: observe Mods execution.
	modsCmd := &cobra.Command{
		Use:                "mods",
		Short:              "Observe Mods execution (logs, events)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleMods(args, stderr)
		},
	}
	root.AddCommand(modsCmd)

	// runs command: inspect and follow individual runs.
	runsCmd := &cobra.Command{
		Use:                "runs",
		Short:              "Inspect and follow individual runs",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRuns(args, stderr)
		},
	}
	root.AddCommand(runsCmd)

	// upload command: upload artifact bundle to a run.
	uploadCmd := &cobra.Command{
		Use:                "upload",
		Short:              "Upload artifact bundle to a run (HTTPS)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpload(args, stderr)
		},
	}
	root.AddCommand(uploadCmd)

	// cluster command: manage local cluster descriptors.
	// Note: no handleCluster in current main.go; this is a placeholder for future wiring.
	// If the command is not implemented, we omit it or provide a stub that returns an error.
	// Checking existing main.go, "cluster" is listed in help but not dispatched.
	// For now, we add a stub that prints an error message.
	clusterCmd := &cobra.Command{
		Use:                "cluster",
		Short:              "Manage local cluster descriptors",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("cluster command not yet implemented")
		},
	}
	root.AddCommand(clusterCmd)

	// config command: inspect or update cluster configuration.
	configCmd := &cobra.Command{
		Use:                "config",
		Short:              "Inspect or update cluster configuration",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleConfig(args, stderr)
		},
	}
	root.AddCommand(configCmd)

	// manifest command: inspect and validate integration manifests.
	manifestCmd := &cobra.Command{
		Use:                "manifest",
		Short:              "Inspect and validate integration manifests",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleManifest(args, stderr)
		},
	}
	root.AddCommand(manifestCmd)

	// knowledge-base command: curate knowledge base fixtures.
	kbCmd := &cobra.Command{
		Use:                "knowledge-base",
		Short:              "Curate knowledge base fixtures",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleKnowledgeBase(args, stderr)
		},
	}
	root.AddCommand(kbCmd)

	// server command: manage control plane server.
	serverCmd := &cobra.Command{
		Use:                "server",
		Short:              "Manage control plane server",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleServer(args, stderr)
		},
	}
	root.AddCommand(serverCmd)

	// node command: manage worker nodes.
	nodeCmd := &cobra.Command{
		Use:                "node",
		Short:              "Manage worker nodes",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleNode(args, stderr)
		},
	}
	root.AddCommand(nodeCmd)

	// rollout command: rolling updates for servers and nodes.
	rolloutCmd := &cobra.Command{
		Use:                "rollout",
		Short:              "Rolling updates for servers and nodes",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRollout(args, stderr)
		},
	}
	root.AddCommand(rolloutCmd)

	// token command: manage API tokens for authentication.
	tokenCmd := &cobra.Command{
		Use:                "token",
		Short:              "Manage API tokens for authentication",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleToken(args, stderr)
		},
	}
	root.AddCommand(tokenCmd)

	// Override help command to preserve existing behavior.
	// Cobra provides a default help command, but we want to preserve printUsage logic.
	// We replace the default help with a custom implementation.
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				printUsage(stderr)
			} else {
				// Dispatch to existing help handlers for subcommands.
				switch args[0] {
				case "mod":
					printModUsage(stderr)
				case "mods":
					printModsUsage(stderr)
				case "runs":
					printRunsUsage(stderr)
				case "server":
					printServerUsage(stderr)
				case "rollout":
					printRolloutUsage(stderr)
				case "config":
					printConfigUsage(stderr)
				case "token":
					printTokenUsage(stderr)
				default:
					printUsage(stderr)
				}
			}
		},
	})

	// Set output to stderr for all cobra messages.
	root.SetOut(stderr)
	root.SetErr(stderr)

	// Configure unknown command handling to match old behavior.
	// When an unknown subcommand is encountered, print usage and return a descriptive error.
	// Cobra's default ValidArgsFunction doesn't apply here; instead, we use a custom approach.
	// We set a flag validator that will be called when cobra encounters an unknown command.
	// However, cobra's built-in unknown command handling already returns an error like "unknown command X for ploy".
	// To also print usage (matching old behavior), we wrap the execution or use a custom error handler.
	// A simpler approach: configure FParseErrWhitelist or use cobra's built-in behavior with custom messages.
	// Let's use cobra's Args field to enforce validation if needed, but cobra already handles unknown commands well.
	// For now, we rely on cobra's default "unknown command" error, which is sufficient.
	// To match the old behavior of printing usage on unknown command, we could use a custom SuggestionsMinimumDistance
	// or intercept in the RunE. But the test expects printUsage to be called.
	// Let's add a FlagErrorFunc to print usage on flag/command errors.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		// Print usage when a flag or command error occurs.
		printUsage(stderr)
		return err
	})

	return root
}

// printVersion outputs version information to the given writer.
// Preserves the existing version format.
func printVersion(w io.Writer) {
	v := iversion.Version
	if strings.TrimSpace(v) == "" {
		v = "dev"
	}
	_, _ = fmt.Fprintf(w, "ploy version %s\n", v)
	if iversion.Commit != "" || iversion.BuiltAt != "" {
		_, _ = fmt.Fprintf(w, "commit %s\n", iversion.Commit)
		if iversion.BuiltAt != "" {
			_, _ = fmt.Fprintf(w, "built  %s\n", iversion.BuiltAt)
		}
	}
}
