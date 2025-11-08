package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	iversion "github.com/iw2rmb/ploy/internal/version"
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
	case "version", "--version", "-version":
		printVersion(stderr)
		return nil
	case "help":
		if len(args) > 1 {
			switch args[1] {
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
			default:
				printUsage(stderr)
			}
		} else {
			printUsage(stderr)
		}
		return nil
	case "mod":
		return handleMod(args[1:], stderr)
	case "upload":
		return handleUpload(args[1:], stderr)
		// environment command is not dispatched in this build; help lists it.
	case "manifest":
		return handleManifest(args[1:], stderr)
	case "knowledge-base":
		return handleKnowledgeBase(args[1:], stderr)
	case "mods":
		return handleMods(args[1:], stderr)
	case "runs":
		return handleRuns(args[1:], stderr)
	case "server":
		return handleServer(args[1:], stderr)
	case "node":
		return handleNode(args[1:], stderr)
	case "rollout":
		return handleRollout(args[1:], stderr)
	case "config":
		return handleConfig(args[1:], stderr)

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
	_, _ = fmt.Fprintln(w, "Ploy CLI v2")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  ploy <command> [<args>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Core Commands:")
	_, _ = fmt.Fprintln(w, "  mod              Plan and run Mods workflows")
	_, _ = fmt.Fprintln(w, "  mods             Observe Mods execution (logs, events)")
	_, _ = fmt.Fprintln(w, "  runs             Inspect and follow individual runs")
	_, _ = fmt.Fprintln(w, "  upload           Upload artifact bundle to a run (HTTPS)")
	_, _ = fmt.Fprintln(w, "  cluster          Manage local cluster descriptors")
	_, _ = fmt.Fprintln(w, "  config           Inspect or update cluster configuration")
	_, _ = fmt.Fprintln(w, "  manifest         Inspect and validate integration manifests")
	_, _ = fmt.Fprintln(w, "  knowledge-base   Curate knowledge base fixtures")
	_, _ = fmt.Fprintln(w, "  server           Manage control plane server")
	_, _ = fmt.Fprintln(w, "  node             Manage worker nodes")
	_, _ = fmt.Fprintln(w, "  rollout          Rolling updates for servers and nodes")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use 'ploy help <command>' for detailed command help.")
}

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
