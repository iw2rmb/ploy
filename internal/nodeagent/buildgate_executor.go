package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// BuildGateExecutor handles execution of build gate validation jobs.
type BuildGateExecutor struct {
	cfg Config
}

// NewBuildGateExecutor creates a new build gate executor.
func NewBuildGateExecutor(cfg Config) *BuildGateExecutor {
	return &BuildGateExecutor{
		cfg: cfg,
	}
}

// Execute runs a build gate validation job.
func (e *BuildGateExecutor) Execute(ctx context.Context, jobID types.JobID, req contracts.BuildGateValidateRequest) (*contracts.BuildGateStageMetadata, error) {
	slog.Info("executing buildgate job", "job_id", jobID)

	// Create ephemeral workspace directory.
	workspaceRoot, err := createWorkspaceDir()
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(workspaceRoot)
	}()

	// Populate workspace via Git clone (repo+ref baseline for validation).
	if err := e.cloneRepo(ctx, req.RepoURL, req.Ref, workspaceRoot); err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}

	// Apply optional diff_patch on top of the cloned baseline.
	// This enables healing flows to verify build fixes by replaying changes
	// (gzipped unified diff) without shipping full workspace archives.
	if len(req.DiffPatch) > 0 {
		slog.Info("applying diff_patch to workspace",
			"job_id", jobID,
			"patch_size", len(req.DiffPatch),
		)
		if err := e.applyDiffPatch(ctx, workspaceRoot, req.DiffPatch); err != nil {
			return nil, fmt.Errorf("apply diff_patch: %w", err)
		}
	}

	// Create gate executor.
	containerRuntime, err := step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
		Network:   os.Getenv("PLOY_DOCKER_NETWORK"),
	})
	if err != nil {
		return nil, fmt.Errorf("create container runtime: %w", err)
	}

	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

	// Build gate spec.
	gateSpec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: req.Profile,
		Env:     make(map[string]string),
	}

	// Execute gate validation.
	metadata, err := gateExecutor.Execute(ctx, gateSpec, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("execute gate: %w", err)
	}

	slog.Info("buildgate job completed",
		"job_id", jobID,
		"passed", len(metadata.StaticChecks) > 0 && metadata.StaticChecks[0].Passed,
	)

	return metadata, nil
}

// cloneRepo clones a git repository into the workspace.
func (e *BuildGateExecutor) cloneRepo(ctx context.Context, repoURL, ref, workspace string) error {
	slog.Info("cloning repository", "repo_url", repoURL, "ref", ref, "workspace", workspace)

	// Create git fetcher with cache support when PLOYD_CACHE_HOME is set.
	// This avoids repeated network fetches for the same repo/ref/commit across runs.
	cacheDir := os.Getenv("PLOYD_CACHE_HOME")
	gitFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{
		PublishSnapshot: false,
		CacheDir:        cacheDir,
	})
	if err != nil {
		return fmt.Errorf("create git fetcher: %w", err)
	}

	// Prepare repo materialization.
	repo := &contracts.RepoMaterialization{
		URL:       types.RepoURL(repoURL),
		TargetRef: types.GitRef(ref),
	}

	// Fetch the repository.
	if err := gitFetcher.Fetch(ctx, repo, workspace); err != nil {
		return fmt.Errorf("fetch repo: %w", err)
	}

	slog.Info("repository cloned successfully", "workspace", workspace)
	return nil
}

// applyDiffPatch applies a gzipped unified diff to the workspace.
// The patch is decompressed and applied via "git apply" to replay changes
// on top of the cloned repo baseline.
//
// This method reuses the applyGzippedPatch helper from execution.go to avoid
// duplication. The shared helper handles:
//   - Gzip decompression of the patch bytes
//   - Empty patch handling (no-op for zero-length patches)
//   - git apply execution with proper error reporting
func (e *BuildGateExecutor) applyDiffPatch(ctx context.Context, workspace string, gzippedPatch []byte) error {
	return applyGzippedPatch(ctx, workspace, gzippedPatch)
}
