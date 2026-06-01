package app

import (
	"fmt"
	"io"
	"strings"

	runcli "github.com/iw2rmb/ploy/internal/cli/run"
	"github.com/spf13/cobra"

	iversion "github.com/iw2rmb/ploy/internal/version"
)

// NewRootCmd constructs the cobra root command with all subcommands.
// It preserves the existing CLI surface and error reporting behavior.
func NewRootCmd(stderr io.Writer) *cobra.Command {
	return NewRootCmdWithIO(stderr, stderr)
}

// NewRootCmdWithIO constructs the cobra root command with explicit stdout/stderr.
func NewRootCmdWithIO(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "ploy",
		Short:         "Ploy CLI v2",
		Long:          "Ploy CLI v2 — control plane and node management",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			versionFlag, _ := cmd.Flags().GetBool("version")
			if versionFlag {
				printVersion(stdout)
				return nil
			}
			return cmd.Help()
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion(stdout)
		},
	}
	root.AddCommand(versionCmd)
	root.Flags().BoolP("version", "v", false, "Print version information")

	root.AddCommand(newMigCmd(stderr))   // ploy mig (run, fetch, cancel, inspect, artifacts, diffs)
	root.AddCommand(runcli.NewCommand()) // ploy run (submit, list, status, cancel, pull, apply)
	root.AddCommand(newWaveCmd(stdout, stderr))
	root.AddCommand(newJobCmd(stderr))  // ploy job (follow job logs)
	root.AddCommand(newPullCmd(stderr)) // ploy pull (local repo pull workflow)

	root.AddCommand(newClusterCmd(stderr))        // ploy cluster (node, token)
	root.AddCommand(newConfigCmd(stdout, stderr)) // ploy config
	root.AddCommand(newSpecCmd(stdout, stderr))

	root.AddCommand(newTUICmd(stderr)) // ploy tui (interactive terminal UI)

	root.SetOut(stdout)
	root.SetErr(stderr)

	return root
}

// printVersion outputs version information to the given writer.
// Preserves the existing version format.
func printVersion(w io.Writer) {
	v := iversion.Version
	if strings.TrimSpace(v) == "" {
		v = "dev"
	}
	_, _ = fmt.Fprintf(w, "%s\n", v)
	if iversion.Commit != "" || iversion.BuiltAt != "" {
		_, _ = fmt.Fprintf(w, "commit %s\n", iversion.Commit)
		if iversion.BuiltAt != "" {
			_, _ = fmt.Fprintf(w, "built  %s\n", iversion.BuiltAt)
		}
	}
}
