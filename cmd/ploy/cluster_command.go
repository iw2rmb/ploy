package main

import (
	"errors"
	"fmt"
	"io"
)

// handleCluster routes cluster subcommands (deploy, node, rollout, token) to
// their respective handlers. This provides a unified namespace for cluster
// management operations under `ploy cluster`.
//
// The cluster command is the primary entry point for:
//   - deploy: Deploy runtime stack on the current host (delegates to handleClusterDeploy)
//   - node:   Manage worker nodes in a cluster (delegates to handleNode)
//   - rollout: Perform rolling updates for servers and nodes (delegates to handleRollout)
//   - token:  Manage API tokens bound to a cluster (delegates to handleToken)
func handleCluster(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	// This mirrors the pattern used by other routers (handleServer, handleNode, etc.)
	if wantsHelp(args) {
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
	case "deploy":
		// Delegate to embedded runtime deployment handler.
		return handleClusterDeploy(args[1:], stderr)
	case "node":
		// Delegate to the existing node handler which supports `add` subcommand.
		return handleNode(args[1:], stderr)
	case "rollout":
		// Delegate to the existing rollout handler which supports `server` and `nodes`.
		return handleRollout(args[1:], stderr)
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
	_, _ = fmt.Fprintln(w, "  deploy   Deploy runtime stack on the current host")
	_, _ = fmt.Fprintln(w, "  node     Manage worker nodes in a cluster")
	_, _ = fmt.Fprintln(w, "  rollout  Perform rolling updates for servers and nodes")
	_, _ = fmt.Fprintln(w, "  token    Manage API tokens bound to a cluster")
}
