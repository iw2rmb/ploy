package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// rehydrateWorkspaceForStep returns a sticky per-(run,repo) workspace for the
// given step. If the sticky workspace already exists and has a valid git dir,
// it is reused as-is; otherwise the workspace is rebuilt from base clone and
// ordered prior diffs.
//
// For step 0: Creates base clone (or reuses cached base if available).
// For step k>0: Copies base clone + applies diffs from steps 0 through k-1.
//
// This function keeps a single mutable workspace per run/repo chain on a node
// while preserving deterministic rebuild when workspace is missing/corrupt.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, commit_sha, and job_name.
//   - manifest: StepManifest for this step.
//
// Returns:
//   - workspacePath: Path to the rehydrated workspace ready for execution.
//   - error: Non-nil if rehydration fails (clone, copy, or patch application error).
func (r *runController) rehydrateWorkspaceForStep(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (string, error) {
	runID := req.RunID.String()
	repoID := req.RepoID
	if repoID.IsZero() {
		return "", fmt.Errorf("rehydrate workspace: repo_id is required for repo-scoped diffs listing")
	}
	workspacePath := runRepoWorkspaceDir(req.RunID, repoID)
	if hasGitDir(workspacePath) {
		slog.Info("reusing sticky workspace", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		return workspacePath, nil
	}
	if _, err := os.Stat(workspacePath); err == nil {
		slog.Warn("sticky workspace is invalid; rebuilding", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		if rmErr := os.RemoveAll(workspacePath); rmErr != nil {
			return "", fmt.Errorf("remove invalid workspace: %w", rmErr)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect sticky workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		return "", fmt.Errorf("create sticky workspace parent dir: %w", err)
	}

	// Step 1: Ensure base clone exists (create on first use, reuse on subsequent calls).
	// Base clone path is deterministic per run and node.
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	baseClone := filepath.Join(baseRoot, "ploy", "run", runID, "base")
	if err := os.MkdirAll(baseClone, 0o755); err != nil {
		return "", fmt.Errorf("create base clone dir: %w", err)
	}

	slog.Info("creating base clone for run", "run_id", runID, "path", baseClone)

	// Initialize git fetcher for repository hydration. The fetcher is responsible for
	// reusing cached clones when PLOYD_CACHE_HOME is configured.
	gitFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{
		CacheDir: os.Getenv("PLOYD_CACHE_HOME"),
	})
	if err != nil {
		return "", fmt.Errorf("create git fetcher: %w", err)
	}

	// Determine repo materialization:
	// - Prefer manifest inputs that already carry hydration.Repo (gate/mig jobs).
	// - Fallback to StartRunRequest repo fields (healing jobs and other callers).
	var repo *contracts.RepoMaterialization
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			repo = input.Hydration.Repo
			break
		}
	}

	if repo == nil {
		// Derive repo materialization from StartRunRequest, mirroring
		// buildManifestFromRequest semantics.
		targetRef := strings.TrimSpace(req.TargetRef.String())
		if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
			targetRef = strings.TrimSpace(req.BaseRef.String())
		}

		tmp := contracts.RepoMaterialization{
			URL:       req.RepoURL,
			BaseRef:   req.BaseRef,
			TargetRef: types.GitRef(targetRef),
			Commit:    req.CommitSHA,
		}
		repo = &tmp
	}

	if err := gitFetcher.Fetch(ctx, repo, baseClone); err != nil {
		return "", fmt.Errorf("hydrate base clone: %w", err)
	}

	slog.Info("base clone created", "run_id", runID, "path", baseClone)

	// Step 2: Rehydrate workspace from base clone + ordered diffs.
	// The reconstructed workspace is persisted under run+repo cache path.
	// C2: For ALL steps (including step 0), fetch diffs and apply them.
	// This ensures step 0 runs on the healed baseline if pre-mig healing occurred.

	slog.Info("rehydrating workspace from base + diffs", "run_id", runID, "job_id", req.JobID)

	diffFetcher, err := NewDiffFetcher(r.cfg)
	if err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// Fetch prior diffs for this repo execution. Current job diff is excluded.
	gzippedDiffs, err := diffFetcher.FetchDiffsForJobRepo(ctx, req.RunID, repoID, req.JobID)
	if err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("fetch diffs for job: %w", err)
	}

	slog.Info("fetched diffs for rehydration", "run_id", runID, "job_id", req.JobID, "diff_count", len(gzippedDiffs))

	// Rehydrate workspace from base + diffs using the helper from execution.go.
	if err := os.RemoveAll(workspacePath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale sticky workspace before rehydration: %w", err)
	}
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, workspacePath, gzippedDiffs); err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("rehydrate from base and diffs: %w", err)
	}

	slog.Info("workspace rehydrated successfully", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)

	// Create baseline commit after rehydration to enable incremental diffs.
	// C2: Now applies to ALL steps (including step 0) when diffs were applied.
	// This commit establishes a new HEAD so that "git diff HEAD" generates
	// only the changes from this step, not cumulative changes from prior steps.
	if len(gzippedDiffs) > 0 {
		if err := ensureBaselineCommitForRehydration(ctx, workspacePath); err != nil {
			if rmErr := os.RemoveAll(workspacePath); rmErr != nil {
				slog.Warn("failed to remove workspace after baseline commit error", "path", workspacePath, "error", rmErr)
			}
			return "", fmt.Errorf("create baseline commit for rehydration: %w", err)
		}
		slog.Info("baseline commit created for incremental diff", "run_id", runID, "job_id", req.JobID)
	}

	return workspacePath, nil
}

func runRepoWorkspaceDir(runID types.RunID, repoID types.MigRepoID) string {
	return filepath.Join(runCacheDir(runID), "repos", repoID.String(), "workspace")
}

func runRepoShareDir(runID types.RunID, repoID types.MigRepoID) string {
	if repoID.IsZero() {
		return ""
	}
	return filepath.Join(runCacheDir(runID), "repos", repoID.String(), "share")
}

func ensureRunRepoShareDir(runID types.RunID, repoID types.MigRepoID) (string, error) {
	shareDir := runRepoShareDir(runID, repoID)
	if strings.TrimSpace(shareDir) == "" {
		return "", nil
	}
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		return "", fmt.Errorf("create run/repo share dir: %w", err)
	}
	return shareDir, nil
}

func hasGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
