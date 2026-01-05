// mod_run_pull.go implements the `ploy mod run pull` subcommand surface for
// pulling Mods diffs into the current git worktree.
//
// This file provides CLI routing, flag parsing, git worktree validation, and
// run resolution for the pull operation. The command enforces:
//   - Execution inside a git repository (via git rev-parse --is-inside-work-tree)
//   - A clean working tree (no staged or unstaged changes via git status --porcelain=v1)
//   - A resolvable git remote (default: "origin" via git remote get-url)
//
// Command structure:
//
//	ploy mod run pull [--origin <remote>] [--dry-run] <run-id>
//
// The origin URL is normalized using vcs.NormalizeRepoURL to enable consistent
// matching against server-side repo identifiers. The normalization trims whitespace
// and strips trailing slashes and .git suffixes.
//
// Run resolution uses the repo-centric API (/v1/repos/{repo_id}/runs) to locate
// the correct run for the current repository and resolves the requested run_id.
//
// The pull workflow then:
//   - Fetches base_ref from the repo snapshot and performs `git fetch <origin> <base_ref> --depth=1`.
//   - Creates the target branch at the fetched commit and checks it out.
//   - Downloads all Mods diffs and applies them via `git apply`, or prints the
//     planned actions when --dry-run is set.
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

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/vcs"
)

// handleModRunPull implements `ploy mod run pull [--origin <remote>] [--dry-run] <run-id>`.
// Parses CLI flags, validates arguments, enforces git worktree preconditions, and resolves the run.
//
// The function performs the following steps in order:
//  1. Parses --origin and --dry-run flags, extracts <run-id> positional argument
//  2. Verifies current directory is inside a git worktree
//  3. Verifies working tree is clean (no staged or unstaged changes)
//  4. Resolves and validates the specified git remote URL
//  5. Calls the repo-centric API to list runs for the repository
//  6. Resolves <run-id> to a unique run
//  7. Uses repo-scoped refs (base_ref/target_ref) and diffs for subsequent steps
//
// Arguments:
//   - args: remaining arguments after "pull" has been stripped (e.g., ["--dry-run", "my-run"])
//   - stderr: writer for user-facing output and error messages
//
// Returns an error if argument parsing fails, preconditions are not met, run resolution fails,
// or git/API operations fail.
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
	// The final positional argument is <run-id>.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModRunPullUsage(stderr)
		return errors.New("run-id required")
	}

	// Extract the run identifier (first non-flag argument).
	runNameOrID := strings.TrimSpace(rest[0])

	// Validate that no extra positional arguments were provided.
	// The command expects exactly one positional argument.
	if len(rest) > 1 {
		printModRunPullUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", rest[1])
	}

	// Create a context with a reasonable timeout for git and API operations.
	// This prevents the command from hanging indefinitely on slow or unresponsive operations.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Normalize the origin URL for comparison/logging purposes.
	// The raw URL is used for API calls to match stored run_repos.repo_url values.
	// Uses the shared vcs.NormalizeRepoURL helper per roadmap/v1/scope.md:28.
	normalizedOriginURL := vcs.NormalizeRepoURL(rawOriginURL)

	// Step 4: Resolve the run via the repo-centric API.
	resolvedRun, err := resolveRunForPull(ctx, rawOriginURL, runNameOrID)
	if err != nil {
		return err
	}

	// Log resolved run information for user visibility.
	_, _ = fmt.Fprintf(stderr, "mod run pull: resolved run %q from origin %q\n", runNameOrID, *origin)
	_, _ = fmt.Fprintf(stderr, "  run ID: %s\n", resolvedRun.RunID)
	_, _ = fmt.Fprintf(stderr, "  repo status: %s\n", resolvedRun.RepoStatus)

	// v1: No per-repo execution run IDs. The resolved RunID is the execution identifier.
	executionRunID := resolvedRun.RunID

	// Base snapshot: use the per-repo base_ref snapshot from RepoRunSummary.
	baseRef := strings.TrimSpace(resolvedRun.BaseRef)
	if baseRef == "" {
		return errors.New("mod run pull: base_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  base ref: %s\n", baseRef)

	// Determine the target branch name.
	targetRef := strings.TrimSpace(resolvedRun.TargetRef)
	if targetRef == "" {
		return errors.New("mod run pull: target_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  target ref: %s\n", targetRef)

	// Step 7: Fetch the base ref from the origin remote and resolve the fetched commit.
	if err := fetchRef(ctx, *origin, baseRef, stderr, *dryRun); err != nil {
		return err
	}
	baseCommit := ""
	if !*dryRun {
		commit, err := resolveFetchHeadSHA(ctx)
		if err != nil {
			return err
		}
		baseCommit = commit
		_, _ = fmt.Fprintf(stderr, "  base commit: %s\n", baseCommit)
	}

	// Step 8: Check for branch collisions (local and remote).
	// Per ROADMAP.md: Check both local and remote for existing branches with the same name.
	if err := checkBranchCollision(ctx, *origin, targetRef, stderr); err != nil {
		return err
	}

	// Step 9: Fetch all diffs for the execution run.
	diffs, err := fetchAllDiffs(ctx, executionRunID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "  diffs to apply: %d\n", len(diffs))

	// Step 10: Handle --dry-run mode.
	// Per ROADMAP.md: For --dry-run, do not execute git branch/checkout/apply calls.
	if *dryRun {
		_, _ = fmt.Fprintf(stderr, "\nWould create branch %q at %q (origin %q) and apply %d Mods diff(s)\n",
			targetRef, baseRef, *origin, len(diffs))
		for i, diff := range diffs {
			_, _ = fmt.Fprintf(stderr, "  diff %d: %s (%d bytes gzipped)\n",
				i+1, diff.ID, diff.Size)
		}
		return nil
	}

	// Step 11: Create the target branch at the fetched base commit.
	if err := createAndCheckoutBranch(ctx, targetRef, baseCommit, stderr); err != nil {
		return err
	}

	// Step 12: Download and apply all diffs.
	// Per ROADMAP.md: For each diff, download, decompress, and apply via `git apply`.
	appliedCount, err := downloadAndApplyDiffs(ctx, diffs, stderr)
	if err != nil {
		return err
	}

	// Success message.
	_, _ = fmt.Fprintf(stderr, "\nApplied %d Mods diff(s) from run %s to branch %q (origin %q)\n",
		appliedCount, executionRunID, targetRef, *origin)
	_, _ = fmt.Fprintf(stderr, "  normalized origin URL: %s\n", normalizedOriginURL)

	return nil
}

// resolveRunForPull fetches runs for the given repository URL from the control plane
// and resolves <run-id> to a unique run.
//
// Parameters:
//   - ctx: context for timeout and cancellation
//   - repoURL: raw repository URL from git remote (used for repo_id resolution)
//   - runNameOrID: the <run-id> argument from CLI
//
// Returns the resolved RepoRunSummary on success, or an error if resolution fails.
func resolveRunForPull(ctx context.Context, repoURL, runNameOrID string) (*mods.RepoRunSummary, error) {
	// Get control plane HTTP client.
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: %w", err)
	}

	// Fetch runs for this repository using the repo-centric API.
	// Use limit=100 per ROADMAP.md specification to cover recent history.
	cmd := mods.ListRunsForRepoCommand{
		Client:  httpClient,
		BaseURL: base,
		RepoURL: repoURL,
		Limit:   100,
		Offset:  0,
	}

	runs, err := cmd.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: failed to list runs for repo: %w", err)
	}

	// If no runs found for this repo, return a clear error.
	if len(runs) == 0 {
		return nil, fmt.Errorf("mod run pull: no runs found for origin %q", repoURL)
	}

	// Resolve the run by name or ID using the resolution rules.
	resolved := mods.ResolveRunForRepo(runs, runNameOrID)
	if resolved == nil {
		// Per ROADMAP.md: return clear error if no matching run found.
		return nil, fmt.Errorf("mod run pull: no run found for %q and origin %q", runNameOrID, repoURL)
	}

	return resolved, nil
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

// printModRunPullUsage renders the usage help for `ploy mod run pull`.
// Follows the pattern of other printModRun*Usage helpers in the codebase.
func printModRunPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run pull [--origin <remote>] [--dry-run] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Pulls Mods diffs from a run into the current git repository.")
	_, _ = fmt.Fprintln(w, "Creates a new branch at the run's base commit and applies stored diffs.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Arguments:")
	_, _ = fmt.Fprintln(w, "  <run-id>  Run ID (KSUID string)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --origin <remote>  Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run          Validate and print actions without mutating the repo")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull --dry-run 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
	_, _ = fmt.Fprintln(w, "  ploy mod run pull --origin upstream 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
}

// =============================================================================
// Diff Retrieval, Branch Creation, and Patch Application Helpers
// =============================================================================

// fetchAllDiffs fetches all diffs for the given execution run.
// Returns diffs sorted by step_index for correct application order.
func fetchAllDiffs(ctx context.Context, executionRunID domaintypes.RunID) ([]mods.DiffEntry, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: %w", err)
	}

	cmd := mods.ListAllDiffsCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   executionRunID,
	}

	diffs, err := cmd.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: failed to list diffs: %w", err)
	}

	return diffs, nil
}

// fetchRef fetches the given ref from the origin remote using a shallow fetch.
// The fetched commit is available as FETCH_HEAD.
func fetchRef(ctx context.Context, origin, ref string, stderr io.Writer, dryRun bool) error {
	_, _ = fmt.Fprintf(stderr, "  fetching %q from %s...\n", ref, origin)
	if dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "fetch", origin, ref, "--depth=1")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		stderrStr := stderrBuf.String()
		if strings.Contains(stderrStr, "couldn't find remote ref") ||
			strings.Contains(stderrStr, "not found") ||
			strings.Contains(stderrStr, "invalid refspec") {
			return fmt.Errorf("mod run pull: ref %q not reachable from origin %q", ref, origin)
		}
		return fmt.Errorf("mod run pull: git fetch failed: %w (stderr: %s)", err, stderrStr)
	}

	return nil
}

func resolveFetchHeadSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "FETCH_HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mod run pull: failed to resolve FETCH_HEAD: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	sha := strings.TrimSpace(stdout.String())
	if sha == "" {
		return "", fmt.Errorf("mod run pull: FETCH_HEAD resolved to empty sha")
	}
	return sha, nil
}

// checkBranchCollision checks if a branch with the given name already exists locally or remotely.
// Per ROADMAP.md: Check both local refs/heads/<target_ref> and remote refs via git ls-remote.
func checkBranchCollision(ctx context.Context, origin, targetRef string, stderr io.Writer) error {
	// Check local branch existence.
	// `git show-ref --verify refs/heads/<target_ref>` returns 0 if branch exists.
	localCmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "refs/heads/"+targetRef)
	localCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	if err := localCmd.Run(); err == nil {
		// Branch exists locally.
		return fmt.Errorf("mod run pull: branch %q already exists locally", targetRef)
	}
	// If error, branch doesn't exist locally (expected).

	// Check remote branch existence.
	// `git ls-remote --heads <origin> <target_ref>` returns the ref if it exists.
	remoteCmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", origin, targetRef)
	remoteCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var remoteBuf bytes.Buffer
	remoteCmd.Stdout = &remoteBuf

	if err := remoteCmd.Run(); err == nil && strings.TrimSpace(remoteBuf.String()) != "" {
		// Branch exists on remote.
		return fmt.Errorf("mod run pull: branch %q already exists on remote %q", targetRef, origin)
	}

	return nil
}

// createAndCheckoutBranch creates a new branch at the given commit and checks it out.
func createAndCheckoutBranch(ctx context.Context, targetRef, commitSHA string, stderr io.Writer) error {
	_, _ = fmt.Fprintf(stderr, "  creating branch %q at %s...\n", targetRef, commitSHA)

	// Step 1: Create the branch.
	branchCmd := exec.CommandContext(ctx, "git", "branch", targetRef, commitSHA)
	branchCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var branchStderr bytes.Buffer
	branchCmd.Stderr = &branchStderr

	if err := branchCmd.Run(); err != nil {
		return fmt.Errorf("mod run pull: failed to create branch %q: %w (stderr: %s)", targetRef, err, branchStderr.String())
	}

	// Step 2: Checkout the branch.
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", targetRef)
	checkoutCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var checkoutStderr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("mod run pull: failed to checkout branch %q: %w (stderr: %s)", targetRef, err, checkoutStderr.String())
	}

	_, _ = fmt.Fprintf(stderr, "  switched to branch %q\n", targetRef)
	return nil
}

// downloadAndApplyDiffs downloads and applies all diffs to the working tree.
// Returns the count of successfully applied diffs (excluding empty patches).
//
// Per ROADMAP.md: For each diff, decompress gzipped patch bytes and apply via `git apply`.
// Empty patches (after trimming whitespace) are skipped, matching applyGzippedPatch semantics.
func downloadAndApplyDiffs(ctx context.Context, diffs []mods.DiffEntry, stderr io.Writer) (int, error) {
	if len(diffs) == 0 {
		return 0, nil
	}

	// Get control plane HTTP client for downloading diffs.
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return 0, fmt.Errorf("mod run pull: %w", err)
	}

	appliedCount := 0

	for i, diff := range diffs {
		_, _ = fmt.Fprintf(stderr, "  applying diff %d/%d: %s (step %d)...\n",
			i+1, len(diffs), diff.ID, diff.StepIndex)

		// Download the diff patch (returns decompressed bytes).
		downloadCmd := mods.DownloadDiffCommand{
			Client:  httpClient,
			BaseURL: base,
			DiffID:  diff.ID,
		}

		patch, err := downloadCmd.Run(ctx)
		if err != nil {
			return appliedCount, fmt.Errorf("mod run pull: failed to download diff %s: %w", diff.ID, err)
		}

		// Skip empty patches (per ROADMAP.md and applyGzippedPatch semantics).
		if len(bytes.TrimSpace(patch)) == 0 {
			_, _ = fmt.Fprintf(stderr, "    skipped (empty patch)\n")
			continue
		}

		// Apply the patch via `git apply`.
		if err := applyPatch(ctx, patch); err != nil {
			return appliedCount, fmt.Errorf("mod run pull: failed to apply diff %s: %w", diff.ID, err)
		}

		appliedCount++
		_, _ = fmt.Fprintf(stderr, "    applied (%d bytes)\n", len(patch))
	}

	return appliedCount, nil
}

// applyPatch applies a unified diff patch to the current working directory via `git apply`.
// Uses the same semantics as internal/nodeagent/execution.go::applyGzippedPatch.
func applyPatch(ctx context.Context, patch []byte) error {
	// git apply without --index applies changes to the working tree only.
	cmd := exec.CommandContext(ctx, "git", "apply")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Stdin = bytes.NewReader(patch)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, stderrBuf.String())
	}

	return nil
}
