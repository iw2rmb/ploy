// run_pull.go implements the `ploy run pull <run-id>` subcommand for
// pulling Migs diffs into the current git worktree.
//
// This command pulls diffs from a specific run into the current git repository.
// The command uses POST /v1/runs/{run_id}/pull to resolve the current repo.
//
// Command structure:
//
//	ploy run pull [--origin <remote>] [--dry-run] <run-id>
//
// The origin URL is normalized using vcs.NormalizeRepoURL to enable consistent
// matching against server-side repo identifiers. The normalization trims whitespace
// and strips trailing slashes and .git suffixes.
//
// The pull workflow:
//  1. Verify execution inside a git repository
//  2. Verify working tree is clean
//  3. Resolve git remote URL for the specified origin
//  4. Call POST /v1/runs/{run_id}/pull to resolve repo execution identifiers
//  5. Verify local HEAD matches run_repos.source_commit_sha
//  6. Download all diffs and apply via git apply
package pull

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// HandleRunPull implements `ploy run pull [--origin <remote>] [--dry-run] <run-id>`.
// Parses CLI flags, validates arguments, enforces git worktree preconditions, and resolves the run.
//
// The command:
//   - Must be executed from inside a git repository
//   - Derives repo identity from git remote URL (origin by default)
//   - Uses POST /v1/runs/{run_id}/pull to resolve repo execution identifiers
//   - Pulls diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs
//
// Arguments:
//   - args: remaining arguments after "pull" has been stripped (e.g., ["--dry-run", "my-run"])
//   - stderr: writer for user-facing output and error messages
//
// Returns an error if argument parsing fails, preconditions are not met, run resolution fails,
// or git/API operations fail.
func HandleRunPull(args []string, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printRunPullUsage(stderr)
		return nil
	}

	// Create a flag set for the pull subcommand.
	// Use ContinueOnError to handle parse errors gracefully and show usage.
	fs := flag.NewFlagSet("run pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress default flag error output; we print custom usage.

	// Define flags:
	// --origin: git remote to match (default "origin")
	// --dry-run: validate and print actions without mutating the repo
	origin := fs.String("origin", "origin", "git remote to match (default origin)")
	dryRun := fs.Bool("dry-run", false, "validate and print actions without mutating the repo")

	// Parse the flags from the provided arguments.
	if err := common.ParseFlagSet(fs, args, func() { printRunPullUsage(stderr) }); err != nil {
		return err
	}

	// After flag parsing, remaining args should contain the run identifier.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunPullUsage(stderr)
		return errors.New("run-id required")
	}

	// Extract the run identifier (first non-flag argument).
	runID := strings.TrimSpace(rest[0])

	// Validate that no extra positional arguments were provided.
	if len(rest) > 1 {
		printRunPullUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", rest[1])
	}

	// Create a context with a reasonable timeout for git and API operations.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Step 1: Verify we are inside a git worktree.
	// Uses shared helper from pull_helpers.go.
	if err := ensureInsideGitWorktree(ctx); err != nil {
		return fmt.Errorf("run pull: %w", err)
	}

	// Step 2: Verify the working tree is clean.
	// Uses shared helper from pull_helpers.go.
	if err := ensureCleanWorkingTree(ctx); err != nil {
		return fmt.Errorf("run pull: %w", err)
	}

	// Step 3: Resolve the git remote URL for the specified origin.
	// Uses shared helper from pull_helpers.go.
	rawOriginURL, err := resolveGitRemoteURL(ctx, *origin)
	if err != nil {
		return fmt.Errorf("run pull: %w", err)
	}

	// Log progress for user visibility.
	_, _ = fmt.Fprintf(stderr, "run pull: resolved origin %q → %s\n", *origin, domaintypes.NormalizeRepoURLSchemless(rawOriginURL))

	// Step 4: Resolve repo execution via POST /v1/runs/{run_id}/pull.
	// This is the v1 API that replaces the legacy repo-centric lookup.
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("run pull: %w", err)
	}

	pullCmd := migs.RunPullCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
		RepoURL: rawOriginURL,
	}
	resolution, err := pullCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("run pull: resolve repo: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "run pull: resolved run %s\n", runID)
	_, _ = fmt.Fprintf(stderr, "  repo ID: %s\n", resolution.RepoID.String())

	// Step 5: Fetch repo details to validate the local source commit.
	repoDetails, err := fetchRunRepoDetails(ctx, httpClient, base, domaintypes.RunID(runID), resolution.RepoID)
	if err != nil {
		return fmt.Errorf("run pull: fetch repo details: %w", err)
	}

	baseRef := strings.TrimSpace(repoDetails.BaseRef)
	if baseRef == "" {
		return errors.New("run pull: base_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  base ref: %s\n", baseRef)

	sourceCommit := strings.TrimSpace(repoDetails.SourceCommitSHA)
	if sourceCommit == "" {
		return errors.New("run pull: source_commit_sha is not available for this run")
	}
	if err := ensureHEADMatchesSource(ctx, sourceCommit); err != nil {
		return fmt.Errorf("run pull: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "  source commit: %s\n", sourceCommit)

	// Step 6: Fetch diffs for this repo execution.
	diffs, err := ListRunRepoDiffs(ctx, httpClient, base, domaintypes.RunID(runID), resolution.RepoID)
	if err != nil {
		return fmt.Errorf("run pull: list diffs: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "  diffs to apply: %d\n", len(diffs))

	// Step 7: Handle --dry-run mode.
	if *dryRun {
		_, _ = fmt.Fprintf(stderr, "\nWould apply %d Migs diff(s) to current worktree at %s\n",
			len(diffs), sourceCommit)
		for i, diff := range diffs {
			_, _ = fmt.Fprintf(stderr, "  diff %d: %s (%d bytes gzipped)\n",
				i+1, diff.ID, diff.Size)
		}
		return nil
	}

	// Step 8: Download and apply all diffs.
	// Uses shared helper from pull_helpers.go.
	appliedCount, err := downloadAndApplyDiffs(ctx, domaintypes.RunID(runID), resolution.RepoID, diffs, stderr)
	if err != nil {
		return fmt.Errorf("run pull: %w", err)
	}

	// Success message.
	_, _ = fmt.Fprintf(stderr, "\nApplied %d Migs diff(s) from run %s to current worktree (origin %q)\n",
		appliedCount, runID, *origin)

	return nil
}

// runRepoDetails holds the repo details needed for pull operations.
// This is a simplified structure containing only the fields we need.
type runRepoDetails struct {
	RepoID          domaintypes.MigRepoID `json:"repo_id"`
	BaseRef         string                `json:"base_ref"`
	SourceCommitSHA string                `json:"source_commit_sha,omitempty"`
	Status          string                `json:"status"`
}

// fetchRunRepoDetails fetches the repo details for a run/repo pair.
// Queries GET /v1/runs/{run_id}/repos and filters by repo_id.
func fetchRunRepoDetails(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) (*runRepoDetails, error) {
	if baseURL == nil {
		return nil, errors.New("base url required")
	}

	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Parse the response to find the repo with matching repo_id.
	var result struct {
		Repos []runRepoDetails `json:"repos"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	for _, repo := range result.Repos {
		if repo.RepoID == repoID {
			return &repo, nil
		}
	}

	return nil, fmt.Errorf("repo %s not found in run %s", repoID.String(), runID.String())
}

// printRunPullUsage renders the usage help for `ploy run pull`.
func printRunPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run pull [--origin <remote>] [--dry-run] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Pulls Migs diffs from a run into the current git repository.")
	_, _ = fmt.Fprintln(w, "Applies stored diffs to the current worktree when HEAD matches the run source commit.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use this command when you have a specific run ID.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Arguments:")
	_, _ = fmt.Fprintln(w, "  <run-id>  Run ID (KSUID string)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --origin <remote>  Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run          Validate and print actions without mutating the repo")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy run pull 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
	_, _ = fmt.Fprintln(w, "  ploy run pull --dry-run 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
	_, _ = fmt.Fprintln(w, "  ploy run pull --origin upstream 2xK9mNpL2pY6jYk3kQwY6a7HkKk")
}
