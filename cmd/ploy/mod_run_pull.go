// mod_run_pull.go implements the `ploy mod run pull` subcommand for pulling
// Mods diffs into the current git worktree.
//
// This file provides CLI routing and flag parsing for the pull operation that
// reconstructs Mods changes locally. The command resolves a run by name or ID,
// verifies the local git environment, and applies stored diffs to a new branch.
//
// Command structure:
//   - ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>
//
// The command requires:
//   - A valid git worktree (must be run inside a git repository)
//   - A clean working tree (no staged or unstaged changes)
//   - A resolvable git remote (default: "origin")
//
// Future implementation will:
//   - Resolve the run via /v1/repos/{repo_id}/runs API
//   - Fetch commit SHA and verify reachability
//   - Create target branch and apply diffs
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// handleModRunPull implements `ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>`.
// Parses CLI flags and validates arguments. The actual pull logic (git operations,
// API calls, diff application) will be implemented in subsequent tasks.
//
// Arguments:
//   - args: remaining arguments after "pull" has been stripped (e.g., ["--dry-run", "my-run"])
//   - stderr: writer for user-facing output and error messages
//
// Returns an error if argument parsing fails or required arguments are missing.
func handleModRunPull(args []string, stderr io.Writer) error {
	// Create a flag set for the pull subcommand.
	// Use ContinueOnError to handle parse errors gracefully and show usage.
	fs := flag.NewFlagSet("mod run pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress default flag error output; we print custom usage.

	// Define flags per ROADMAP.md specification:
	// --origin: git remote to match (default "origin")
	// --dry-run: validate and print actions without mutating the repo
	origin := fs.String("origin", "origin", "git remote to match (default origin)")
	dryRun := fs.Bool("dry-run", false, "validate and print actions without mutating the repo")

	// Parse the flags from the provided arguments.
	if err := fs.Parse(args); err != nil {
		printModRunPullUsage(stderr)
		return err
	}

	// After flag parsing, remaining args should contain the run identifier.
	// The final positional argument is <run-name|run-id>.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModRunPullUsage(stderr)
		return errors.New("run-name or run-id required")
	}

	// Extract the run identifier (first non-flag argument).
	runNameOrID := strings.TrimSpace(rest[0])

	// Validate that no extra positional arguments were provided.
	// The command expects exactly one positional argument.
	if len(rest) > 1 {
		printModRunPullUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", rest[1])
	}

	// At this point, we have valid parsed arguments:
	// - runNameOrID: the run to pull diffs from
	// - *origin: the git remote to use (default "origin")
	// - *dryRun: whether to perform a dry run
	//
	// The actual implementation (git worktree detection, API calls, diff application)
	// will be added in subsequent ROADMAP tasks. For now, we just validate the CLI surface.

	// Placeholder: print what would be done (will be replaced with actual logic).
	_, _ = fmt.Fprintf(stderr, "mod run pull: would pull run %q from origin %q (dry-run: %v)\n",
		runNameOrID, *origin, *dryRun)

	// Mark variables as used to satisfy the compiler.
	// These will be consumed by the actual implementation.
	_ = origin
	_ = dryRun
	_ = runNameOrID

	return nil
}

// printModRunPullUsage renders the usage help for `ploy mod run pull`.
// Follows the pattern of other printModRun*Usage helpers in the codebase.
func printModRunPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Pulls Mods diffs from a run into the current git repository.")
	_, _ = fmt.Fprintln(w, "Creates a new branch at the run's base commit and applies stored diffs.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Arguments:")
	_, _ = fmt.Fprintln(w, "  <run-name|run-id>  Name or ID of the Mods run to pull")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --origin <remote>  Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run          Validate and print actions without mutating the repo")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull java17-fleet")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull --dry-run my-batch")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull --origin upstream 2xK9mNpL")
}
