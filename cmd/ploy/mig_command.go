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
	// Mig run with repo selection.
	// Check if first arg looks like a mig ID/name (not a flag) to route to project run.
	case "run":
		// If next arg exists and doesn't start with '-', it's a mig reference.
		if len(args) > 1 && len(args[1]) > 0 && args[1][0] != '-' {
			// Check if it's not a known subcommand (repo, pull).
			switch args[1] {
			case "pull":
				return errors.New("mig run pull has been removed; use 'ploy run pull <run-id>' or 'ploy mig pull'")
			case "repo":
				// Fall through to existing handleMigRun which routes these.
			default:
				// Treat as mig reference for project run.
				return handleMigRunProject(args[1:], stderr)
			}
		}
		return handleMigRun(args[1:], stderr)
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
	_, _ = fmt.Fprintln(w, "Flags for 'ploy mig run':")
	_, _ = fmt.Fprintln(w, "  --spec <file>              Path to YAML/JSON spec file (CLI flags override spec values)")
	_, _ = fmt.Fprintln(w, "  --repo-url <url>           Git repository URL")
	_, _ = fmt.Fprintln(w, "  --repo-base-ref <branch>   Git base ref")
	_, _ = fmt.Fprintln(w, "  --repo-target-ref <branch> Git target ref")
	_, _ = fmt.Fprintln(w, "  --repo-workspace-hint <dir> Optional subdirectory hint")
	_, _ = fmt.Fprintln(w, "  --job-env KEY=VALUE        Job environment (repeatable)")
	_, _ = fmt.Fprintln(w, "  --job-image <image>        Container image for mig step")
	_, _ = fmt.Fprintln(w, "  --job-command <cmd>        Container command override")
	_, _ = fmt.Fprintln(w, "  --gitlab-pat <token>       GitLab Personal Access Token")
	_, _ = fmt.Fprintln(w, "  --gitlab-domain <domain>   GitLab domain")
	_, _ = fmt.Fprintln(w, "  --mr-success               Create merge request on success")
	_, _ = fmt.Fprintln(w, "  --mr-fail                  Create merge request on failure")
	_, _ = fmt.Fprintln(w, "  --follow                   Follow run logs until completion")
	_, _ = fmt.Fprintln(w, "  --cap <duration>           Time cap for --follow")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap            Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --artifact-dir <dir>       Download final artifacts")
	_, _ = fmt.Fprintln(w, "  --json                     Print JSON summary")
	_, _ = fmt.Fprintln(w, "  --max-retries N            Max reconnect attempts for event stream")
}
