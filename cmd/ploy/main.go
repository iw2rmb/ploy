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

	// Legacy command aliases removed; treat unknown names uniformly.

	switch args[0] {
	case "help":
		printUsage(stderr)
		return nil
	case "mod":
		return handleMod(args[1:], stderr)
	case "upload":
		return handleUpload(args[1:], stderr)
	case "report":
		return handleReport(args[1:], stderr)
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
	case "server":
		return handleServer(args[1:], stderr)
	case "node":
		return handleNode(args[1:], stderr)

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
	_, _ = fmt.Fprintln(w, "Ploy CLI")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  ploy <command> [<args>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Available Commands:")
	_, _ = fmt.Fprintln(w, "  server           Manage server deployment")
	_, _ = fmt.Fprintln(w, "  node             Manage node deployment")
	_, _ = fmt.Fprintln(w, "  mods             Manage mods")
	_, _ = fmt.Fprintln(w, "  jobs             Manage jobs/runs")
	_, _ = fmt.Fprintln(w, "  mod              Work with mod configurations")
	_, _ = fmt.Fprintln(w, "  upload           Upload artifacts")
	_, _ = fmt.Fprintln(w, "  report           Generate reports")
	_, _ = fmt.Fprintln(w, "  environment      Manage environments")
	_, _ = fmt.Fprintln(w, "  manifest         Work with manifests")
	_, _ = fmt.Fprintln(w, "  knowledge-base   Manage knowledge base")
}
