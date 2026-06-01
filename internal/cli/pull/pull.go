// pull.go implements the `ploy pull` command for the local repo pull workflow.
//
// This command:
//   - Ensures a Migs run exists for the current local repo HEAD SHA
//   - Pulls the resulting diffs into the local git worktree
//
// Command structure:
//
//	ploy pull [--new-run] [--follow] [--origin <remote>] [--dry-run]
//
// Behavior:
//   - Maintains per-repo pull state that binds {repo_url, head_sha, run_id, created_at}
//   - If no saved pull state: initiates a run and requires --follow or re-invocation
//   - If SHA mismatch: requires --new-run to initiate a fresh run
//   - If SHA matches and run is terminal-success: pulls diffs
//   - If --follow is set: follows run until terminal and then pulls diffs
package pull

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	climigs "github.com/iw2rmb/ploy/internal/cli/migs"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// pullState represents the persisted state for a pull operation.
// Stored in <git-dir>/ploy/pull_state.json.
type pullState struct {
	RepoURL   string    `json:"repo_url"`
	HeadSHA   string    `json:"head_sha"`
	RunID     string    `json:"run_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Handle implements `ploy pull [--new-run] [--follow] [--origin <remote>] [--dry-run]`.
// See cmd/ploy/README.md for user-facing behavior.
func Handle(args []string, stderr io.Writer) error {
	// Handle help flag.
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printPullUsage(stderr)
			return nil
		}
	}

	// Parse flags.
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	newRun := fs.Bool("new-run", false, "Force initiating a new run")
	followFlag := fs.Bool("follow", false, "Follow run until completion")
	origin := fs.String("origin", "origin", "Git remote to match")
	dryRun := fs.Bool("dry-run", false, "Validate and print actions without mutating")

	// Follow mode flags.
	capDuration := fs.Duration("cap", 0, "Optional time cap for --follow")
	cancelOnCap := fs.Bool("cancel-on-cap", false, "Cancel run if cap exceeded")
	maxRetries := fs.Int("max-retries", 5, "Max SSE reconnect attempts")

	if err := fs.Parse(args); err != nil {
		printPullUsage(stderr)
		return err
	}

	// No positional args expected.
	if len(fs.Args()) > 0 {
		printPullUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", fs.Args()[0])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: Verify we are inside a git worktree.
	if err := ensureInsideGitWorktree(ctx); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Step 2: Verify working tree is clean.
	if err := ensureCleanWorkingTree(ctx); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Step 3: Resolve git remote URL.
	rawOriginURL, err := resolveGitRemoteURL(ctx, *origin)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "pull: resolved origin %q → %s\n", *origin, domaintypes.NormalizeRepoURLSchemless(rawOriginURL))

	// Step 4: Get current HEAD SHA.
	headSHA, err := resolveHeadSHA(ctx)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "pull: current HEAD %s\n", headSHA)

	// Step 5: Get control plane connection.
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Step 6: Load pull state.
	gitDir, err := resolveGitDir(ctx)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	statePath := filepath.Join(gitDir, "ploy", "pull_state.json")
	state, stateExists := loadPullState(statePath)

	// Step 7: Decide based on state and flags.
	var runID domaintypes.RunID

	switch {
	case *newRun:
		// --new-run: always initiate a new run.
		if *dryRun {
			_, _ = fmt.Fprintln(stderr, "pull: --dry-run: would initiate a new run (--new-run)")
			_, _ = fmt.Fprintln(stderr, "\nDry run complete. No run was initiated and no state was saved.")
			return nil
		}
		_, _ = fmt.Fprintln(stderr, "pull: initiating new run (--new-run)")
		newRunID, err := initiatePullRun(ctx, httpClient, base, rawOriginURL, stderr)
		if err != nil {
			return err
		}
		runID = newRunID
		// Save new state.
		newState := pullState{
			RepoURL:   rawOriginURL,
			HeadSHA:   headSHA,
			RunID:     runID.String(),
			CreatedAt: time.Now().UTC(),
		}
		if err := savePullState(statePath, newState); err != nil {
			return fmt.Errorf("pull: save state: %w", err)
		}
		_, _ = fmt.Fprintf(stderr, "pull: initiated run %s\n", runID)

	case !stateExists:
		// No saved state: must initiate a run.
		if *dryRun {
			_, _ = fmt.Fprintln(stderr, "pull: --dry-run: no saved pull state; would initiate a run")
			_, _ = fmt.Fprintln(stderr, "\nDry run complete. No run was initiated and no state was saved.")
			return nil
		}
		_, _ = fmt.Fprintln(stderr, "pull: no saved pull state; initiating run")
		newRunID, err := initiatePullRun(ctx, httpClient, base, rawOriginURL, stderr)
		if err != nil {
			return err
		}
		runID = newRunID
		// Save new state.
		newState := pullState{
			RepoURL:   rawOriginURL,
			HeadSHA:   headSHA,
			RunID:     runID.String(),
			CreatedAt: time.Now().UTC(),
		}
		if err := savePullState(statePath, newState); err != nil {
			return fmt.Errorf("pull: save state: %w", err)
		}
		_, _ = fmt.Fprintf(stderr, "pull: initiated run %s\n", runID)

		if !*followFlag {
			return fmt.Errorf("run initiated (%s); rerun with --follow to wait for completion, or inspect status with `ploy run status %s`", runID, runID)
		}

	case state.HeadSHA != headSHA:
		// SHA mismatch: require --new-run.
		return fmt.Errorf("pull: current HEAD %s does not match saved run HEAD %s; rerun with --new-run to initiate a new run", headSHA, state.HeadSHA)

	default:
		// SHA matches: reuse existing run.
		runID = domaintypes.RunID(state.RunID)
		_, _ = fmt.Fprintf(stderr, "pull: reusing run %s (state SHA matches)\n", runID)
	}

	// Step 8: Check run status.
	statusCmd := runs.GetStatusCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   runID,
	}
	summary, err := statusCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("pull: get run status: %w", err)
	}

	runState, err := mapRunSummaryToRunState(summary)
	if err != nil {
		return fmt.Errorf("pull: map run status: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "pull: run status: %s\n", strings.ToLower(string(runState)))

	// Step 9: Handle non-terminal runs.
	if !isTerminalRunState(runState) {
		if !*followFlag {
			return fmt.Errorf("run %s is still %s; rerun with --follow to wait for completion", runID, strings.ToLower(string(runState)))
		}

		// Follow until terminal.
		_, _ = fmt.Fprintln(stderr, "pull: following run until completion...")
		finalState, err := followPullRun(ctx, base, httpClient, runID, *capDuration, *cancelOnCap, *maxRetries, stderr)
		if err != nil {
			return err
		}
		runState = finalState
	}

	// Step 10: Check final state.
	if runState != migsapi.RunStateSucceeded {
		return fmt.Errorf("run ended in %s; cannot pull diffs", strings.ToLower(string(runState)))
	}

	// Step 11: Pull diffs (reuse run pull logic).
	_, _ = fmt.Fprintln(stderr, "pull: run succeeded; pulling diffs...")
	return executePullDiffs(ctx, httpClient, base, runID, rawOriginURL, *origin, *dryRun, stderr)
}

// resolveHeadSHA returns the current HEAD commit SHA.
func resolveHeadSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", errors.New("HEAD resolved to empty sha")
	}
	return sha, nil
}

// resolveGitDir returns the path to the .git directory.
func resolveGitDir(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return "", errors.New("git dir resolved to empty path")
	}
	// Convert to absolute path if relative.
	if !filepath.IsAbs(gitDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get cwd: %w", err)
		}
		gitDir = filepath.Join(cwd, gitDir)
	}
	return gitDir, nil
}

// loadPullState loads pull state from the given path.
// Returns the state and true if it exists, or empty state and false if not.
// Logs a warning if the file exists but contains invalid JSON.
func loadPullState(path string) (pullState, bool) {
	data, err := common.ReadFileRooted(path)
	if err != nil {
		return pullState{}, false
	}
	var state pullState
	if err := json.Unmarshal(data, &state); err != nil {
		// File exists but is corrupt - log warning so users know state was ignored.
		_, _ = fmt.Fprintf(os.Stderr, "pull: warning: corrupt state file %s (ignored): %v\n", path, err)
		return pullState{}, false
	}
	return state, true
}

// savePullState saves pull state to the given path.
func savePullState(path string, state pullState) error {
	// Ensure directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// initiatePullRun creates a mig-project run for the current repo.
// Infers the mig from the repo URL and creates a run scoped to only this repo.
func initiatePullRun(ctx context.Context, httpClient *http.Client, baseURL *url.URL, repoURL string, stderr io.Writer) (domaintypes.RunID, error) {
	// Infer mig from repo.
	migID, err := inferMigFromRepo(ctx, httpClient, baseURL, repoURL, stderr)
	if err != nil {
		return "", fmt.Errorf("pull: %w", err)
	}

	// Create mig-project run scoped to this repo.
	cmd := climigs.CreateMigRunCommand{
		Client:   httpClient,
		BaseURL:  baseURL,
		MigRef:   domaintypes.MigRef(migID),
		RepoURLs: []string{repoURL},
	}
	result, err := cmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("pull: create run: %w", err)
	}

	return firstRunIDForWave(ctx, httpClient, baseURL, result.WaveID)
}

func firstRunIDForWave(ctx context.Context, httpClient *http.Client, baseURL *url.URL, waveID domaintypes.WaveID) (domaintypes.RunID, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.JoinPath("v1", "waves", waveID.String(), "runs").String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return "", httpx.WrapError("pull: list wave runs", resp.Status, resp.Body)
	}
	var payload struct {
		Runs []struct {
			RunID domaintypes.RunID `json:"run_id"`
		} `json:"runs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &payload, httpx.MaxJSONBodyBytes); err != nil {
		return "", err
	}
	if len(payload.Runs) == 0 || payload.Runs[0].RunID.IsZero() {
		return "", fmt.Errorf("wave %s has no runs", waveID)
	}
	return payload.Runs[0].RunID, nil
}

// followPullRun follows the run until terminal state.
func followPullRun(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, capDuration time.Duration, cancelOnCap bool, maxRetries int, stderr io.Writer) (migsapi.RunState, error) {
	followCtx := ctx
	var cancel context.CancelFunc
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}
	renderOpts := common.FollowRunRenderOptions(baseURL, stderr)

	final, err := runs.FollowRunCommand{
		Client:     common.CloneForStream(client),
		BaseURL:    baseURL,
		RunID:      runID,
		Output:     stderr,
		EnableOSC8: renderOpts.EnableOSC8,
		AuthToken:  renderOpts.AuthToken,
		MaxRetries: maxRetries,
	}.Run(followCtx)
	if err != nil {
		// Handle timeout with optional cancellation.
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if cancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  stderr,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(stderr, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return "", nil
		}
		return "", err
	}

	return final, nil
}

// executePullDiffs resolves and applies diffs from the run.
// Reuses the logic from run_pull.go.
func executePullDiffs(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoURL, origin string, dryRun bool, stderr io.Writer) error {
	// Resolve repo execution via POST /v1/runs/{run_id}/repos/resolve.
	pullCmd := climigs.RunPullCommand{
		Client:  httpClient,
		BaseURL: baseURL,
		RunID:   runID,
		RepoURL: repoURL,
	}
	resolution, err := pullCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("pull: resolve repo: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "  repo ID: %s\n", resolution.RepoID.String())

	// Fetch repo details to validate the local source commit.
	repoDetails, err := fetchRunRepoDetails(ctx, httpClient, baseURL, runID, resolution.RepoID)
	if err != nil {
		return fmt.Errorf("pull: fetch repo details: %w", err)
	}

	baseRef := strings.TrimSpace(repoDetails.BaseRef)
	if baseRef == "" {
		return errors.New("pull: base_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  base ref: %s\n", baseRef)

	sourceCommit := strings.TrimSpace(repoDetails.SourceCommitSHA)
	if sourceCommit == "" {
		return errors.New("pull: source_commit_sha is not available for this run")
	}
	if err := ensureHEADMatchesSource(ctx, sourceCommit); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "  source commit: %s\n", sourceCommit)

	// Fetch diffs for this repo execution.
	diffs, err := ListRunRepoDiffs(ctx, httpClient, baseURL, runID, resolution.RepoID)
	if err != nil {
		return fmt.Errorf("pull: list diffs: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "  diffs to apply: %d\n", len(diffs))

	// Handle --dry-run mode.
	if dryRun {
		_, _ = fmt.Fprintf(stderr, "\nWould apply %d Migs diff(s) to current worktree at %s\n",
			len(diffs), sourceCommit)
		for i, diff := range diffs {
			_, _ = fmt.Fprintf(stderr, "  diff %d: %s (%d bytes gzipped)\n",
				i+1, diff.ID, diff.Size)
		}
		return nil
	}

	// Download and apply all diffs.
	appliedCount, err := downloadAndApplyDiffs(ctx, runID, resolution.RepoID, diffs, stderr)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Success message.
	_, _ = fmt.Fprintf(stderr, "\nApplied %d Migs diff(s) from run %s to current worktree (origin %q)\n",
		appliedCount, runID, origin)

	return nil
}

// isTerminalRunState returns true if the run state is terminal.
func isTerminalRunState(s migsapi.RunState) bool {
	switch s {
	case migsapi.RunStateSucceeded, migsapi.RunStateFailed, migsapi.RunStateError, migsapi.RunStateCancelled:
		return true
	}
	return false
}

func mapRunSummaryToRunState(summary domaintypes.RunSummary) (migsapi.RunState, error) {
	switch summary.Status {
	case domaintypes.RunStatusQueued, domaintypes.RunStatusRunning:
		return migsapi.RunStateRunning, nil
	case domaintypes.RunStatusCancelled:
		return migsapi.RunStateCancelled, nil
	case domaintypes.RunStatusSuccess:
		return migsapi.RunStateSucceeded, nil
	case domaintypes.RunStatusFail:
		return migsapi.RunStateFailed, nil
	default:
		return "", fmt.Errorf("unknown run status %q", summary.Status)
	}
}

// printPullUsage prints usage for the pull command.
func printPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy pull [--new-run] [--follow] [--origin <remote>] [--dry-run]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Ensures a Migs run exists for the current local repo HEAD and pulls diffs.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Maintains per-repo pull state that binds {repo_url, head_sha, run_id}.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Behavior:")
	_, _ = fmt.Fprintln(w, "  - If no saved pull state: initiates a run (requires --follow or re-invocation)")
	_, _ = fmt.Fprintln(w, "  - If HEAD SHA mismatch: requires --new-run to initiate a fresh run")
	_, _ = fmt.Fprintln(w, "  - If SHA matches and run succeeded: pulls diffs")
	_, _ = fmt.Fprintln(w, "  - If --follow is set: follows run until terminal and then pulls diffs")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --new-run           Force initiating a new run")
	_, _ = fmt.Fprintln(w, "  --follow            Follow run until completion (shows job graph)")
	_, _ = fmt.Fprintln(w, "  --origin <remote>   Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run           Validate and print actions without mutating")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Follow mode:")
	_, _ = fmt.Fprintln(w, "  --cap <duration>    Optional time cap for --follow (e.g., 30m, 1h)")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap     Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>   Max SSE reconnect attempts (default: 5)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy pull --follow              # Initiate/reuse run, follow, and pull diffs")
	_, _ = fmt.Fprintln(w, "  ploy pull --new-run --follow    # Force new run, follow, and pull diffs")
	_, _ = fmt.Fprintln(w, "  ploy pull --dry-run             # Show what would happen without mutating")
	_, _ = fmt.Fprintln(w, "  ploy pull                       # Pull diffs if run already succeeded")
}
