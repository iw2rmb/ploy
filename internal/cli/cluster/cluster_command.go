package cluster

import (
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"io"
)

// Handle routes node and token subcommands to their respective handlers under
// the existing `ploy cluster` command group.
//
// The cluster command is the primary entry point for:
//   - node:   Manage worker nodes (delegates to handleNode)
//   - token:  Manage API tokens (delegates to handleToken)
func Handle(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	// This mirrors the pattern used by other routers (handleServer, handleNode, etc.)
	if common.WantsHelp(args) {
		printClusterUsage(stderr)
		return nil
	}

	// If no subcommand is provided, print usage and return an error.
	if len(args) == 0 {
		printClusterUsage(stderr)
		return errors.New("cluster subcommand required")
	}

	// Route to the appropriate handler based on the subcommand.
	switch args[0] {
	case "node":
		// Delegate to the existing node handler which supports `add` subcommand.
		return handleNode(args[1:], stderr)
	case "token":
		// Delegate to the existing token handler which supports `create`, `list`, `revoke`.
		return handleToken(args[1:], stderr)
	default:
		// Unknown subcommand: print usage and return an error.
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
