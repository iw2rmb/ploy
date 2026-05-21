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

// prepareStickyWorkspaceForStep returns the single mutable per-(run,repo)
// workspace for a linear repo chain. The chain head hydrates sources directly
// into the sticky workspace; later jobs require that workspace to already exist
// on this node.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, commit_sha, and job_name.
//   - manifest: StepManifest for this step.
//
// Returns:
//   - workspacePath: Path to the sticky workspace ready for execution.
//   - error: Non-nil if workspace preparation fails.
func (r *runController) prepareStickyWorkspaceForStep(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (string, error) {
	runID := req.RunID.String()
	repoID := req.RepoID
	if repoID.IsZero() {
		return "", fmt.Errorf("prepare sticky workspace: repo_id is required")
	}
	workspacePath := runRepoWorkspaceDir(req.RunID, repoID)
	if hasGitDir(workspacePath) {
		slog.Info("reusing sticky workspace", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		return workspacePath, nil
	}

	if req.JobType != types.JobTypePreGate {
		return "", fmt.Errorf("sticky workspace missing for %s job; linear repo chains must continue on the node that hydrated the chain head", req.JobType)
	}

	if _, err := os.Stat(workspacePath); err == nil {
		slog.Warn("sticky workspace is invalid; rebuilding chain head", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		if rmErr := os.RemoveAll(workspacePath); rmErr != nil {
			return "", fmt.Errorf("remove invalid workspace: %w", rmErr)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect sticky workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o750); err != nil {
		return "", fmt.Errorf("create sticky workspace parent dir: %w", err)
	}

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

	if err := os.RemoveAll(workspacePath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale sticky workspace before hydration: %w", err)
	}
	if err := gitFetcher.Fetch(ctx, repo, workspacePath); err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("hydrate sticky workspace: %w", err)
	}

	slog.Info("sticky workspace hydrated successfully", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
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
	if err := os.MkdirAll(shareDir, 0o750); err != nil {
		return "", fmt.Errorf("create run/repo share dir: %w", err)
	}
	return shareDir, nil
}

func hasGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
