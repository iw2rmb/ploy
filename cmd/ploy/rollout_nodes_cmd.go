package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// rolloutNodesHost is an indirection to allow tests to stub remote commands.
var rolloutNodesHost = executeRolloutNode

// rolloutNodesAPIClient allows tests to inject a mock HTTP client and base URL.
// Defined in rollout_nodes_api.go.

func handleRolloutNodes(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRolloutNodesUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("rollout nodes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		all         boolValue
		selector    stringValue
		concurrency intValue
		binary      stringValue
		identity    stringValue
		userFlag    stringValue
		sshPort     intValue
		timeout     intValue
		dryRun      boolValue
		maxAttempts intValue
	)

	fs.Var(&all, "all", "Roll out all nodes in the cluster")
	fs.Var(&selector, "selector", "Node name pattern (e.g., 'worker-*')")
	fs.Var(&concurrency, "concurrency", "Number of nodes to update per batch (default: 1)")
	fs.Var(&binary, "binary", "Path to the ployd-node binary for upload (default: alongside the CLI)")
	fs.Var(&identity, "identity", "SSH private key used for node connection (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username for node connection (default: root)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node connection (default: 22)")
	fs.Var(&timeout, "timeout", "Timeout in seconds per node rollout (default: 90)")
	fs.Var(&dryRun, "dry-run", "Print planned rollout actions per node without making changes")
	fs.Var(&maxAttempts, "max-attempts", "Maximum retry attempts for each node (default: 3)")

	if err := parseFlagSet(fs, args, func() { printRolloutNodesUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printRolloutNodesUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	// Validate that either --all or --selector is provided.
	if !all.value && !selector.set {
		printRolloutNodesUsage(stderr)
		return errors.New("either --all or --selector is required")
	}
	if all.value && selector.set {
		printRolloutNodesUsage(stderr)
		return errors.New("--all and --selector are mutually exclusive")
	}

	cfg := rolloutNodesConfig{
		All:          all.value,
		Selector:     selector.value,
		Concurrency:  concurrency.value,
		BinaryPath:   binary.value,
		User:         userFlag.value,
		IdentityFile: identity.value,
		SSHPort:      sshPort.value,
		Timeout:      timeout.value,
		DryRun:       dryRun.value,
		MaxAttempts:  maxAttempts.value,
	}

	return runRolloutNodes(cfg, stderr)
}

// printRolloutNodesUsage prints the rollout nodes subcommand usage information.
// NOTE: Rollout nodes is now accessible via `ploy cluster rollout nodes`.
func printRolloutNodesUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster rollout nodes [--all | --selector <pattern>] [flags]")
}
