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
	case "workflow":
		return handleWorkflow(args[1:], stderr)
	case "mod":
		return handleMod(args[1:], stderr)
	case "lanes":
		return handleLanes(args[1:], stderr)
	case "snapshot":
		return handleSnapshot(args[1:], stderr)
	case "environment":
		return handleEnvironment(args[1:], stderr)
	case "manifest":
		return handleManifest(args[1:], stderr)
	case "knowledge-base":
		return handleKnowledgeBase(args[1:], stderr)
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
	_, _ = fmt.Fprintln(w, "Usage: ploy <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  workflow  Manage workflow execution entries")
	_, _ = fmt.Fprintln(w, "  mod       Execute Mods-specific workflows")
	_, _ = fmt.Fprintln(w, "  lanes     Inspect lane definitions and cache previews")
	_, _ = fmt.Fprintln(w, "  snapshot  Plan and capture database snapshots")
	_, _ = fmt.Fprintln(w, "  environment  Materialize commit-scoped environments")
	_, _ = fmt.Fprintln(w, "  manifest  Inspect integration manifest assets")
	_, _ = fmt.Fprintln(w, "  knowledge-base  Manage knowledge base incidents")
}
