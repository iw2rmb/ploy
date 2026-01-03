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
//	ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>
//
// The origin URL is normalized using normalizeRepoURLForCLI to enable consistent
// matching against server-side repo identifiers. The normalization strips trailing
// slashes and .git suffixes, matching the semantics in internal/worker/hydration/git_fetcher.go.
//
// Run resolution uses the repo-centric API (/v1/repos/{repo_id}/runs) to locate
// the correct run for the current repository, honoring both UUIDs and human-readable
// run names while selecting the first matching result.
//
// The pull workflow then:
//   - Uses RepoRunSummary.ExecutionRunID to locate the execution Mods run.
//   - Fetches commit_sha, base_ref, and target_ref for the execution run.
//   - Verifies commit reachability via `git fetch <origin> <commit_sha> --depth=1`.
//   - Creates the target branch at the pinned commit and checks it out.
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
)

// handleModRunPull implements `ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>`.
// Parses CLI flags, validates arguments, enforces git worktree preconditions, and resolves the run.
//
// The function performs the following steps in order:
//  1. Parses --origin and --dry-run flags, extracts <run-name|run-id> positional argument
//  2. Verifies current directory is inside a git worktree
//  3. Verifies working tree is clean (no staged or unstaged changes)
//  4. Resolves and validates the specified git remote URL
//  5. Calls the repo-centric API to list runs for the repository
//  6. Resolves <run-name|run-id> to a unique run, preferring RunID match over Name match
//  7. Captures ExecutionRunID, base_ref, target_ref, and attempt for subsequent steps
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
	normalizedOriginURL := normalizeRepoURLForCLI(rawOriginURL)

	// Step 4: Resolve the run via the repo-centric API.
	// Use the raw origin URL as repo_id (URL-encoded for path segment) to match
	// the stored run_repos.repo_url value populated via CreateRunRepoParams.RepoUrl.
	resolvedRun, err := resolveRunForPull(ctx, rawOriginURL, runNameOrID)
	if err != nil {
		return err
	}

	// Log resolved run information for user visibility.
	_, _ = fmt.Fprintf(stderr, "mod run pull: resolved run %q from origin %q\n", runNameOrID, *origin)
	if resolvedRun.Name != nil && *resolvedRun.Name != "" {
		_, _ = fmt.Fprintf(stderr, "  run name: %s\n", *resolvedRun.Name)
	}
	_, _ = fmt.Fprintf(stderr, "  run ID: %s\n", resolvedRun.RunID)
	_, _ = fmt.Fprintf(stderr, "  repo status: %s\n", resolvedRun.RepoStatus)

	// Step 5: Validate ExecutionRunID is set.
	// Per ROADMAP.md: If ExecutionRunID is nil or empty, return a clear error.
	if resolvedRun.ExecutionRunID == nil || strings.TrimSpace(*resolvedRun.ExecutionRunID) == "" {
		return fmt.Errorf("mod run pull: execution run id missing for %q (repo may not have started)", runNameOrID)
	}
	executionRunID := domaintypes.RunID(strings.TrimSpace(*resolvedRun.ExecutionRunID))
	_, _ = fmt.Fprintf(stderr, "  execution run ID: %s\n", executionRunID.String())

	// Step 6: Fetch run details to obtain commit_sha.
	// Use the execution run ID to get the run record with commit_sha.
	runDetails, err := fetchRunDetails(ctx, executionRunID)
	if err != nil {
		return err
	}

	// Surface repository URL and base_ref from the execution run for diagnostics.
	if strings.TrimSpace(runDetails.RepoURL) != "" {
		_, _ = fmt.Fprintf(stderr, "  run repository: %s\n", runDetails.RepoURL)
	}
	if strings.TrimSpace(runDetails.BaseRef) != "" {
		_, _ = fmt.Fprintf(stderr, "  base ref: %s\n", runDetails.BaseRef)
	}

	// Validate that commit_sha is available.
	// Per ROADMAP.md: If commit_sha is empty, treat as a hard error.
	if runDetails.CommitSHA == nil || strings.TrimSpace(*runDetails.CommitSHA) == "" {
		return errors.New("mod run pull: commit_sha is not available for this run; pull requires a pinned commit")
	}
	commitSHA := strings.TrimSpace(*runDetails.CommitSHA)
	_, _ = fmt.Fprintf(stderr, "  commit SHA: %s\n", commitSHA)

	// Determine the target branch name.
	// Per ROADMAP.md: Prefer the per-repo target_ref from RepoRunSummary.TargetRef when present;
	// otherwise, fall back to the execution run's target_ref from RunDetails.
	targetRef := resolvedRun.TargetRef
	if strings.TrimSpace(targetRef) == "" {
		targetRef = runDetails.TargetRef
	}
	if strings.TrimSpace(targetRef) == "" {
		return errors.New("mod run pull: target_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  target ref: %s\n", targetRef)

	// Step 7: Verify commit SHA reachability on the origin remote.
	// Per ROADMAP.md: Run `git fetch <origin> <commit_sha> --depth=1`.
	if err := verifyCommitReachable(ctx, *origin, commitSHA, stderr, *dryRun); err != nil {
		return err
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
		_, _ = fmt.Fprintf(stderr, "\nWould create branch %q at %s (origin %q) and apply %d Mods diff(s)\n",
			targetRef, commitSHA, *origin, len(diffs))
		for i, diff := range diffs {
			_, _ = fmt.Fprintf(stderr, "  diff %d: %s (%d bytes gzipped)\n",
				i+1, diff.ID, diff.Size)
		}
		return nil
	}

	// Step 11: Create the target branch at the commit SHA.
	// Per ROADMAP.md: git branch <target_ref> <commit_sha>, then git checkout <target_ref>.
	if err := createAndCheckoutBranch(ctx, targetRef, commitSHA, stderr); err != nil {
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
// and resolves <run-name|run-id> to a unique run using the resolution rules from ROADMAP.md:
//  1. First, try to match by RunID (exact string equality).
//  2. If no RunID match, match against Name (after trimming spaces).
//  3. Filter only runs whose RepoStatus indicates execution ran (succeeded, failed, skipped).
//  4. If multiple results match the same name, select the first entry (API returns DESC by created_at).
//  5. If no match found, return error.
//
// Parameters:
//   - ctx: context for timeout and cancellation
//   - repoURL: raw repository URL from git remote (used as repo_id in API call)
//   - runNameOrID: the <run-name|run-id> argument from CLI
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
//	"ssh://git@github.com/org/repo.git" -> "ssh://git@github.com/org/repo"
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

// =============================================================================
// Diff Retrieval, Branch Creation, and Patch Application Helpers
// =============================================================================

// fetchRunDetails fetches the full run record including commit_sha via the API.
// This uses the runs endpoint (GET /v1/runs/{id}) which exposes commit_sha.
func fetchRunDetails(ctx context.Context, runID domaintypes.RunID) (*mods.RunDetails, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: %w", err)
	}

	cmd := mods.FetchRunWithCommitSHA{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
	}

	details, err := cmd.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod run pull: failed to fetch run details: %w", err)
	}

	return details, nil
}

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

// verifyCommitReachable verifies that the given commit SHA is reachable from the origin remote.
// Uses `git fetch <origin> <commit_sha> --depth=1` to fetch the specific commit.
//
// Per ROADMAP.md: If fetch fails with "couldn't find remote ref" or similar, return an error.
func verifyCommitReachable(ctx context.Context, origin, commitSHA string, stderr io.Writer, dryRun bool) error {
	_, _ = fmt.Fprintf(stderr, "  verifying commit %s is reachable from %s...\n", commitSHA, origin)

	// Run: git fetch <origin> <commit_sha> --depth=1
	// This fetches the specific commit without a full clone.
	cmd := exec.CommandContext(ctx, "git", "fetch", origin, commitSHA, "--depth=1")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		// Check for common error patterns indicating the commit is not reachable.
		stderrStr := stderrBuf.String()
		if strings.Contains(stderrStr, "couldn't find remote ref") ||
			strings.Contains(stderrStr, "not found") ||
			strings.Contains(stderrStr, "invalid refspec") {
			return fmt.Errorf("mod run pull: commit %s not reachable from origin %q (force-push or mirror mismatch)", commitSHA, origin)
		}
		// Generic error.
		return fmt.Errorf("mod run pull: failed to verify commit reachability: %w (stderr: %s)", err, stderrStr)
	}

	return nil
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

// createAndCheckoutBranch creates a new branch at the given commit SHA and checks it out.
// Per ROADMAP.md: `git branch <target_ref> <commit_sha>` followed by `git checkout <target_ref>`.
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
