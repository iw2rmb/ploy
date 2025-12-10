// mod_run_pull.go implements the `ploy mod run pull` subcommand surface for
// pulling Mods diffs into the current git worktree.
//
// This file provides CLI routing, flag parsing, and git worktree validation
// for the pull operation. The command enforces:
//   - Execution inside a git repository (via git rev-parse --is-inside-work-tree)
//   - A clean working tree (no staged or unstaged changes via git status --porcelain=v1)
//   - A resolvable git remote (default: "origin" via git remote get-url)
//
// Command structure:
//
//	ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>
//
// The origin URL is normalized using normalizeRepoURLForCLI to enable consistent
// matching against server-side repo identifiers. The normalization strips trailing
// slashes and .git suffixes, matching the semantics in internal/worker/hydration/git_fetcher.go.
//
// Future implementation will:
//   - Resolve the run via /v1/repos/{repo_id}/runs API
//   - Fetch commit SHA and verify reachability
//   - Create target branch and apply diffs
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// handleModRunPull implements `ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>`.
// Parses CLI flags, validates arguments, and enforces git worktree preconditions.
//
// The function performs the following validations in order:
//  1. Parses --origin and --dry-run flags, extracts <run-name|run-id> positional argument
//  2. Verifies current directory is inside a git worktree
//  3. Verifies working tree is clean (no staged or unstaged changes)
//  4. Resolves and validates the specified git remote URL
//
// Arguments:
//   - args: remaining arguments after "pull" has been stripped (e.g., ["--dry-run", "my-run"])
//   - stderr: writer for user-facing output and error messages
//
// Returns an error if argument parsing fails, preconditions are not met, or
// git operations fail.
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

	// Create a context with a reasonable timeout for git operations.
	// This prevents the command from hanging indefinitely on slow or unresponsive git operations.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Verify we are inside a git worktree.
	// This is a prerequisite for all subsequent git operations.
	if err := ensureInsideGitWorktree(ctx); err != nil {
		return err
	}

	// Step 2: Verify the working tree is clean (no staged or unstaged changes).
	// This prevents accidental data loss and ensures deterministic patch application.
	if err := ensureCleanWorkingTree(ctx); err != nil {
		return err
	}

	// Step 3: Resolve the git remote URL for the specified origin.
	// This validates that the remote exists and returns the raw URL for API matching.
	rawOriginURL, err := resolveGitRemoteURL(ctx, *origin)
	if err != nil {
		return err
	}

	// Normalize the origin URL for comparison with server-side repo identifiers.
	// The raw URL is preserved for exact matching where needed (e.g., API calls).
	normalizedOriginURL := normalizeRepoURLForCLI(rawOriginURL)

	// Placeholder: print what would be done (will be replaced with actual API and diff logic).
	_, _ = fmt.Fprintf(stderr, "mod run pull: would pull run %q from origin %q (dry-run: %v)\n",
		runNameOrID, *origin, *dryRun)
	_, _ = fmt.Fprintf(stderr, "  raw origin URL: %s\n", rawOriginURL)
	_, _ = fmt.Fprintf(stderr, "  normalized origin URL: %s\n", normalizedOriginURL)

	return nil
}

// ensureInsideGitWorktree verifies that the current working directory is inside
// a git repository worktree. Uses git rev-parse --is-inside-work-tree which
// returns "true" when inside a worktree and fails otherwise.
//
// Environment variables GIT_TERMINAL_PROMPT=0 and GIT_ASKPASS=echo are set to
// prevent interactive credential prompts, matching the pattern used in
// internal/worker/hydration/git_fetcher.go::runGitCommand.
func ensureInsideGitWorktree(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	// Disable interactive prompts to avoid hanging in headless/CI environments.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// git rev-parse fails when not inside a git repository.
		// Return a user-friendly error message per ROADMAP.md specification.
		return errors.New("mod run pull: must be run inside a git repository")
	}

	// git rev-parse --is-inside-work-tree outputs "true" when inside a worktree.
	// In rare edge cases (e.g., bare repos), it may output "false".
	output := strings.TrimSpace(stdout.String())
	if output != "true" {
		return errors.New("mod run pull: must be run inside a git repository")
	}

	return nil
}

// ensureCleanWorkingTree verifies that the git working tree has no staged or
// unstaged changes. Uses git status --porcelain=v1 which outputs nothing when
// the working tree is clean.
//
// This check prevents accidental data loss during branch operations and ensures
// that patch application starts from a known state.
func ensureCleanWorkingTree(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1")
	// Disable interactive prompts for consistency with other git operations.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// git status failure is unexpected; wrap the error with context.
		return fmt.Errorf("mod run pull: failed to check working tree status: %w", err)
	}

	// Porcelain v1 format outputs one line per changed file.
	// Any non-empty output indicates uncommitted changes.
	output := stdout.String()
	if len(output) > 0 {
		return errors.New("mod run pull: working tree must be clean (commit or stash changes first)")
	}

	return nil
}

// resolveGitRemoteURL retrieves the URL for the specified git remote.
// Uses git remote get-url which outputs the configured URL for the remote.
//
// Parameters:
//   - ctx: context for timeout and cancellation
//   - remoteName: name of the git remote (e.g., "origin", "upstream")
//
// Returns the raw remote URL string on success, or an error if the remote
// does not exist or git operations fail.
func resolveGitRemoteURL(ctx context.Context, remoteName string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
	// Disable interactive prompts for consistency with other git operations.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// git remote get-url fails when the remote does not exist.
		// Return a user-friendly error message per ROADMAP.md specification.
		return "", fmt.Errorf("mod run pull: git remote %q not found", remoteName)
	}

	// The remote URL is the trimmed stdout output.
	rawURL := strings.TrimSpace(stdout.String())
	if rawURL == "" {
		// Edge case: remote exists but has no URL configured (should be rare).
		return "", fmt.Errorf("mod run pull: git remote %q has no URL configured", remoteName)
	}

	return rawURL, nil
}

// normalizeRepoURLForCLI normalizes a git repository URL for comparison with
// server-side repo identifiers. The normalization removes artifacts that may
// differ between equivalent URLs:
//   - Trailing whitespace
//   - Trailing slashes
//   - Trailing .git suffix
//
// This matches the normalization semantics in internal/worker/hydration/git_fetcher.go::normalizeRepoURL.
//
// The raw URL should be preserved for exact matching where required (e.g., when
// calling /v1/repos/{repo_id}/runs where repo_id must match stored run_repos.repo_url).
//
// Examples:
//
//	"https://github.com/org/repo.git"  -> "https://github.com/org/repo"
//	"https://github.com/org/repo/"    -> "https://github.com/org/repo"
//	"git@github.com:org/repo.git"     -> "git@github.com:org/repo"
func normalizeRepoURLForCLI(raw string) string {
	normalized := strings.TrimSpace(raw)
	normalized = strings.TrimSuffix(normalized, "/")
	normalized = strings.TrimSuffix(normalized, ".git")
	return normalized
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
