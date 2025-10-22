package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// main bootstraps the CLI entrypoint.
func main() {
	if err := execute(os.Args[1:], os.Stderr); err != nil {
		reportError(err, os.Stderr)
		os.Exit(1)
	}
}

// execute routes the top-level command to the appropriate handler.
func execute(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("command required")
	}

	switch args[0] {
	case "help":
		return handleHelp(args[1:], stderr)
	case "workflow":
		return handleWorkflow(args[1:], stderr)
	case "mod":
		return handleMod(args[1:], stderr)
	case "artifact":
		return handleArtifact(args[1:], stderr)
	case "config":
		return handleConfig(args[1:], stderr)
	case "cluster":
		return handleCluster(args[1:], stderr)
	case "snapshot":
		return handleSnapshot(args[1:], stderr)
	case "environment":
		return handleEnvironment(args[1:], stderr)
	case "manifest":
		return handleManifest(args[1:], stderr)
	case "knowledge-base":
		return handleKnowledgeBase(args[1:], stderr)
	case "mods":
		return handleMods(args[1:], stderr)
	case "jobs":
		return handleJobs(args[1:], stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// reportError prints a standard error prefix for CLI failures.
func reportError(err error, stderr io.Writer) {
	_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
}

// printUsage lists the available top-level commands.
func printUsage(w io.Writer) {
	lines := []string{
		"Ploy CLI v2",
		"",
		"Usage:",
		"  ploy <command> [<args>]",
		"",
		"Core Commands:",
		"  mod         Plan and run Mods workflows",
		"  mods        Observe Mods execution (logs, events)",
		"  jobs        Inspect and follow individual jobs",
		"  artifact    Manage IPFS Cluster artifacts",
		"  node        Administer Ploy nodes and lifecycle",
		"  deploy      Bootstrap or upgrade clusters",
		"  cluster     Manage local cluster descriptors",
		"  beacon      Control beacon discovery operations",
		"  config      Inspect or update cluster configuration",
		"  status      Summarize cluster health",
		"  doctor      Run workstation diagnostics",
		"",
		"Use 'ploy help <command>' for detailed command help.",
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

func handleHelp(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return nil
	}
	switch args[0] {
	case "mod":
		printModUsage(stderr)
		return nil
	case "artifact":
		printArtifactUsage(stderr)
		return nil
	case "node":
		printNodeUsage(stderr)
		return nil
	case "deploy":
		printDeployUsage(stderr)
		return nil
	case "cluster":
		printClusterUsage(stderr)
		return nil
	case "beacon":
		printBeaconUsage(stderr)
		return nil
	case "config":
		printConfigUsage(stderr)
		return nil
	case "mods":
		printModsUsage(stderr)
		return nil
	case "jobs":
		printJobsUsage(stderr)
		return nil
	case "status":
		printStatusUsage(stderr)
		return nil
	case "doctor":
		printDoctorUsage(stderr)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown help topic %q", args[0])
	}
}
