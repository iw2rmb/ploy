package main

import (
	"errors"
	"fmt"
	"io"
)

// handleMig routes mig subcommands to their implementations.
func handleMig(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printMigUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printMigUsage(stderr)
		return errors.New("mig subcommand required")
	}

	switch args[0] {
	// Mig management commands.
	case "add":
		return handleMigAdd(args[1:], stderr)
	case "list":
		return handleMigList(args[1:], stderr)
	case "remove":
		return handleMigRemove(args[1:], stderr)
	case "archive":
		return handleMigArchive(args[1:], stderr)
	case "unarchive":
		return handleMigUnarchive(args[1:], stderr)
	// Spec management.
	case "spec":
		return handleMigSpec(args[1:], stderr)
	// Repo set management.
	case "repo":
		return handleMigRepo(args[1:], stderr)
	// Pull command: pulls diffs from a mig's latest run into the current repo.
	case "pull":
		return handleMigPull(args[1:], stderr)
	case "status":
		return handleMigStatus(args[1:], stderr)
	// Mig run with repo selection.
	case "run":
		if len(args) <= 1 {
			printMigRunProjectUsage(stderr)
			return errors.New("mig id/name required")
		}
		if args[1] == "--help" || args[1] == "-h" {
			printMigRunProjectUsage(stderr)
			return nil
		}
		switch args[1] {
		case "pull":
			return errors.New("mig run pull has been removed; use 'ploy run pull <run-id>' or 'ploy mig pull'")
		case "repo":
			return handleMigRunRepo(args[2:], stderr)
		default:
			if len(args[1]) > 0 && args[1][0] == '-' {
				printMigRunProjectUsage(stderr)
				return errors.New("mig id/name required")
			}
			return handleMigRunProject(args[1:], stderr)
		}
	case "fetch":
		return handleMigFetch(args[1:], stderr)
	case "artifacts":
		return handleMigArtifacts(args[1:], stderr)
	default:
		printMigUsage(stderr)
		return fmt.Errorf("unknown mig subcommand %q", args[0])
	}
}

func printMigUsage(w io.Writer) {
	printCommandUsage(w, "mig")
	// Append a concise flags summary for 'ploy mig run' to match help golden.
	// Keep this in sync with cmd/ploy/testdata/help_mig.txt.
	_, _ = fmt.Fprintln(w, "")
	printMigRunFlagsSummary(w)
}

// printMigRunFlagsSummary renders the 'ploy mig run' flags overview used by 'help mig'.
// This is a human-friendly summary, not the exhaustive FlagSet help.
func printMigRunFlagsSummary(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Flags for 'ploy mig run <mig>':")
	_, _ = fmt.Fprintln(w, "  --repo <url>    Explicit repo URL(s) to run (repeatable)")
	_, _ = fmt.Fprintln(w, "  --failed        Run repos with last terminal state Fail")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags for 'ploy mig pull':")
	_, _ = fmt.Fprintln(w, "  --origin <remote>    Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run            Validate and print actions without mutating the repo")
	_, _ = fmt.Fprintln(w, "  --last-failed        Select the latest failed run (default: last succeeded)")
	_, _ = fmt.Fprintln(w, "  --last-succeeded     Select the latest succeeded run (default)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Single-repo runs now live under 'ploy run':")
	_, _ = fmt.Fprintln(w, "  ploy run --repo <url> --base-ref <ref> --target-ref <ref> --spec <path|->")
}
