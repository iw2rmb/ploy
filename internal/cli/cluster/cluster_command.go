package cluster

import (
	"errors"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

// Handle routes commands under the `ploy cluster` group.
func Handle(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printClusterUsage(stderr)
		return nil
	}

	if len(args) == 0 {
		printClusterUsage(stderr)
		return errors.New("cluster subcommand required")
	}

	switch args[0] {
	case "node":
		return handleNode(args[1:], stdout, stderr)
	case "token":
		return handleToken(args[1:], stderr)
	default:
		printClusterUsage(stderr)
		return fmt.Errorf("unknown cluster subcommand %q", args[0])
	}
}

// printClusterUsage prints the cluster command usage information.
// This provides a single, consistent usage output for --help, error paths,
// and unknown subcommand handling.
func printClusterUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  node     Manage worker nodes")
	_, _ = fmt.Fprintln(w, "  token    Manage API tokens")
}
