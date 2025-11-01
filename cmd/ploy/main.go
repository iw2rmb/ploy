package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iw2rmb/ploy/internal/clitree"
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
		return handleHelp(args[1:], stderr)
	case "mod":
		return handleMod(args[1:], stderr)
	case "artifact":
		return handleArtifact(args[1:], stderr)
	case "upload":
		return handleUpload(args[1:], stderr)
	case "report":
		return handleReport(args[1:], stderr)
	case "config":
		return handleConfig(args[1:], stderr)
	case "cluster":
		return handleCluster(args[1:], stderr)

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
	nodes := clitree.Tree()
	visible := make([]clitree.Node, 0, len(nodes))
	width := 0
	for _, node := range nodes {
		if node.Hidden {
			continue
		}
		visible = append(visible, node)
		if l := len(node.Name); l > width {
			width = l
		}
	}
	padding := width + 2

	_, _ = fmt.Fprintln(w, "Ploy CLI v2")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  ploy <command> [<args>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Core Commands:")
	for _, node := range visible {
		desc := node.Description
		_, _ = fmt.Fprintf(w, "  %-*s %s\n", padding, node.Name, desc)
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use 'ploy help <command>' for detailed command help.")
}

func handleHelp(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return nil
	}

	if node, ok := clitree.Lookup(args...); ok {
		renderNodeUsage(stderr, node, args)
		return nil
	}

	if node, ok := clitree.Lookup(args[0]); ok {
		renderNodeUsage(stderr, node, args[:1])
		return fmt.Errorf("unknown help topic %q", clitree.PathString(args...))
	}

	printUsage(stderr)
	return fmt.Errorf("unknown help topic %q", args[0])
}
