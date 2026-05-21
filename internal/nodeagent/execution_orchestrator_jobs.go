// execution_orchestrator_jobs.go contains mig job implementations,
// the shared standard job executor, and workspace lifecycle helpers.
package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// executeMigJob runs a mig container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mig steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeMigJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// MigStackUnknown which falls back to "default" in stack maps.
	stack := resolveManifestStack(req, r.loadPersistedStack(req.RunID))

	// Build manifest with stack-aware image resolution using typed options.
	typedOpts := req.TypedOptions
	stepIdx := 0
	if len(typedOpts.Steps) > 0 {
		if req.MigContext != nil {
			stepIdx = req.MigContext.StepIndex
		} else {
			idx, err := migStepIndexFromJobName(req.JobName, len(typedOpts.Steps))
			if err != nil {
				err = fmt.Errorf("derive mig step index from job_name: %w", err)
				slog.Error("failed to derive mig step index", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
				return
			}
			stepIdx = idx
		}
		if stepIdx < 0 || stepIdx >= len(typedOpts.Steps) {
			err := fmt.Errorf("derived mig step index out of range: derived=%d steps_len=%d", stepIdx, len(typedOpts.Steps))
			slog.Error("derived mig step index out of range", "run_id", req.RunID, "job_id", req.JobID, "derived_index", stepIdx, "steps_len", len(typedOpts.Steps))
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
	}
	manifest, err := buildManifestFromRequest(req, typedOpts, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mig job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest: manifest,
		DiffType: types.DiffJobTypeMig,
		PopulateInDir: func(inDir string) error {
			return r.materializeMigInFromInputs(ctx, req, inDir)
		},
		UploadConfiguredArtifacts: true,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, workspace string, result step.Result, diffPath string) (bool, error) {
			return r.uploadJobDiff(ctx, runID, jobID, diffGen, workspace, result, types.DiffJobTypeMig, diffPath)
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// standardJobConfig configures the execution of a standard container job.
type standardJobConfig struct {
	Manifest contracts.StepManifest
	DiffType types.DiffJobType

	PopulateInDir   func(inDir string) error
	PrepareManifest func(manifest *contracts.StepManifest, workspace string) error
	RuntimeSync     func(outDir, workspace string) error
	ValidateOutputs func(outDir, workspace string) error
	FinalizeOutputs func(outDir, workspace string) error
	TrySkip         func(ctx context.Context, manifest contracts.StepManifest, workspace, outDir string) (bool, error)

	UploadConfiguredArtifacts bool

	UploadDiff    func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, workspace string, result step.Result, diffPath string) (bool, error)
	BuildJobMeta  func(outDir string) json.RawMessage
	BuildMetadata func(outDir string) map[string]string

	SuppressTerminalStatus bool
	SuppressOutBundle      bool

	StartTime time.Time
}

type standardJobOutcome struct {
	runErr     error
	result     step.Result
	repoSHAOut string
	duration   time.Duration
}

// executeStandardJob orchestrates the common lifecycle of a container job:
// runtime init, sticky workspace preparation, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	outcome, execErr := r.executeStandardJobWithOutcome(ctx, req, cfg)
	if execErr == nil {
		if outcome.runErr != nil || outcome.result.ExitCode != 0 {
			r.uploadRepoArtifactsIfPresent(req.RunID, req.RepoID, req.JobID)
		}
		return
	}
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	slog.Error("standard job execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", execErr)
	r.uploadRepoArtifactsIfPresent(req.RunID, req.RepoID, req.JobID)
	r.uploadFailureStatus(ctx, req, execErr, time.Since(startTime))
}

func (r *runController) executeStandardJobWithOutcome(ctx context.Context, req StartRunRequest, cfg standardJobConfig) (standardJobOutcome, error) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	var outcome standardJobOutcome

	artifactPaths := runRepoJobArtifactPaths(req.RunID, req.RepoID, req.JobID)
	if err := ensureJobArtifactDirs(artifactPaths); err != nil {
		return outcome, fmt.Errorf("prepare job artifacts: %w", err)
	}

	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		return outcome, fmt.Errorf("initialize runtime: %w", err)
	}
	defer cleanup()
	artifactLogs, err := newArtifactLogWriter(execCtx.logStreamer, artifactPaths)
	if err != nil {
		return outcome, fmt.Errorf("prepare job artifact logs: %w", err)
	}
	execCtx.runner.LogWriter = artifactLogs
	defer func() {
		if err := artifactLogs.Close(); err != nil {
			slog.Warn("failed to close job artifact logs", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	}()

	wsResult, err := r.prepareStickyWorkspaceWithCleanup(ctx, req, cfg.Manifest)
	if err != nil {
		return outcome, fmt.Errorf("prepare sticky workspace: %w", err)
	}
	defer wsResult.cleanup()
	workspace := wsResult.path

	if cfg.PopulateInDir != nil {
		if err := cfg.PopulateInDir(artifactPaths.In); err != nil {
			return outcome, fmt.Errorf("populate in dir: %w", err)
		}
	}
	stepOutcome, err := r.runContainerJob(ctx, req, cfg, execCtx, workspace, startTime, artifactPaths.Out, artifactPaths.In, artifactPaths.Diff)
	if err != nil {
		return outcome, err
	}
	outcome = stepOutcome
	return outcome, nil
}

// runContainerJob executes the container, uploads artifacts/diffs, and reports terminal status.
// Extracted from executeStandardJob to keep function sizes under ~100 lines.
func (r *runController) runContainerJob(
	ctx context.Context,
	req StartRunRequest,
	cfg standardJobConfig,
	execCtx jobExecutionContext,
	workspace string,
	startTime time.Time,
	outDir, inDir, diffPath string,
) (standardJobOutcome, error) {
	outcome := standardJobOutcome{}
	shareDir, err := ensureRunRepoShareDir(req.RunID, req.RepoID)
	if err != nil {
		return outcome, err
	}
	manifest := cfg.Manifest
	disableManifestGate(&manifest)
	clearManifestHydration(&manifest)

	if cfg.PrepareManifest != nil {
		if err := cfg.PrepareManifest(&manifest, workspace); err != nil {
			return outcome, fmt.Errorf("prepare manifest: %w", err)
		}
	}

	imageName := strings.TrimSpace(manifest.Image)
	if imageName == "" {
		return outcome, fmt.Errorf("resolved job image is empty")
	}
	if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
		return outcome, fmt.Errorf("save job image name: %w", err)
	}

	var result step.Result
	var runErr error
	var duration time.Duration
	if cfg.TrySkip != nil {
		skipped, err := cfg.TrySkip(ctx, manifest, workspace, outDir)
		if err != nil {
			return outcome, fmt.Errorf("evaluate skip: %w", err)
		}
		if skipped {
			duration := time.Since(startTime)
			if runErr == nil && cfg.ValidateOutputs != nil {
				if validateErr := cfg.ValidateOutputs(outDir, workspace); validateErr != nil {
					runErr = fmt.Errorf("validate job outputs: %w", validateErr)
				}
			}
			runErr = r.finalizeStandardJobOutputs(req, cfg, outDir, workspace, runErr, step.Result{})
			repoSHAOut := ""
			if runErr == nil {
				var repoSHAErr error
				repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, "")
				if repoSHAErr != nil {
					runErr = repoSHAErr
					slog.Error("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "error", repoSHAErr)
				}
			}
			statsBuilder := types.NewRunStatsBuilder().
				ExitCode(0).
				DurationMs(duration.Milliseconds())
			if runErr != nil {
				statsBuilder.Error(normalizedExecutionError(runErr))
			}
			stats := statsBuilder.MustBuild()
			if !cfg.SuppressOutBundle {
				if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, outDir, "mig-out"); err != nil {
					slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
				}
			}
			outcome = standardJobOutcome{
				runErr:     runErr,
				result:     step.Result{},
				repoSHAOut: repoSHAOut,
				duration:   duration,
			}
			if !cfg.SuppressTerminalStatus {
				r.reportTerminalStatus(ctx, req, runErr, step.Result{}, stats, repoSHAOut, duration)
			}
			return outcome, nil
		}
	}

	preWorkspaceTree := ""
	if tree, treeErr := gitpkg.ComputeWorkspaceTreeSHA(ctx, workspace); treeErr != nil {
		return outcome, fmt.Errorf("compute pre-execution workspace tree: %w", treeErr)
	} else {
		preWorkspaceTree = tree
	}

	// Materialize Hydra resources into a staging directory for mount planning.
	stopRuntimeSync := r.startRuntimeOutputSyncLoop(ctx, req, cfg, outDir, workspace)
	if bundleErr := r.withMaterializedResources(ctx, manifest, req.TypedOptions.BundleMap, "ploy-staging-*", func(stagingDir string) error {
		result, runErr = execCtx.runner.Run(ctx, step.Request{
			RunID:      req.RunID,
			JobID:      req.JobID,
			Manifest:   manifest,
			Workspace:  workspace,
			OutDir:     outDir,
			InDir:      inDir,
			ShareDir:   shareDir,
			StagingDir: stagingDir,
		})
		duration = time.Since(startTime)
		return nil
	}); bundleErr != nil {
		stopRuntimeSync()
		return outcome, bundleErr
	}
	stopRuntimeSync()

	if runErr == nil && result.ExitCode == 0 && cfg.ValidateOutputs != nil {
		if validateErr := cfg.ValidateOutputs(outDir, workspace); validateErr != nil {
			runErr = fmt.Errorf("validate job outputs: %w", validateErr)
		}
	}
	runErr = r.finalizeStandardJobOutputs(req, cfg, outDir, workspace, runErr, result)
	duration = time.Since(startTime)

	diffUploaded := false
	if cfg.UploadDiff != nil {
		var diffErr error
		diffUploaded, diffErr = cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, workspace, result, diffPath)
		if diffErr != nil && runErr == nil && result.ExitCode == 0 {
			runErr = fmt.Errorf("upload job diff: %w", diffErr)
		}
	}

	if runErr == nil && result.ExitCode == 0 && req.JobType == types.JobTypeMig {
		if err := advanceWorkspaceBaseline(ctx, workspace, req.RunID, req.JobID, diffUploaded); err != nil {
			runErr = fmt.Errorf("advance workspace baseline: %w", err)
			slog.Error("failed to advance workspace baseline", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	}

	if !cfg.SuppressOutBundle {
		if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, outDir, "mig-out"); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
		}
	}

	if cfg.UploadConfiguredArtifacts {
		r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace, outDir)
	}

	repoSHAOut := ""
	if runErr == nil && result.ExitCode == 0 {
		var repoSHAErr error
		repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, preWorkspaceTree)
		if repoSHAErr != nil {
			runErr = repoSHAErr
			slog.Error("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "error", repoSHAErr)
		}
	}

	statsBuilder := types.NewRunStatsBuilder().
		ExitCode(result.ExitCode).
		DurationMs(duration.Milliseconds()).
		TimingsFromDurations(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		)
	if resources := runStatsJobResourcesFromStepUsage(result.ContainerResources); resources != nil {
		statsBuilder.JobResources(resources)
	}

	if cfg.BuildJobMeta != nil {
		if meta := cfg.BuildJobMeta(outDir); len(meta) > 0 {
			statsBuilder.JobMeta(meta)
		}
	}
	if cfg.BuildMetadata != nil {
		for k, v := range cfg.BuildMetadata(outDir) {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			statsBuilder.MetadataEntry(k, v)
		}
	}
	if runErr != nil {
		statsBuilder.Error(normalizedExecutionError(runErr))
	}

	stats := statsBuilder.MustBuild()
	outcome = standardJobOutcome{
		runErr:     runErr,
		result:     result,
		repoSHAOut: repoSHAOut,
		duration:   duration,
	}
	if !cfg.SuppressTerminalStatus {
		r.reportTerminalStatus(ctx, req, runErr, result, stats, repoSHAOut, duration)
	}
	return outcome, nil
}

func (r *runController) finalizeStandardJobOutputs(
	req StartRunRequest,
	cfg standardJobConfig,
	outDir, workspace string,
	runErr error,
	result step.Result,
) error {
	if cfg.FinalizeOutputs == nil {
		return runErr
	}
	if finalizeErr := cfg.FinalizeOutputs(outDir, workspace); finalizeErr != nil {
		// Keep non-zero container exits mapped to their original fail/error
		// semantics while still attempting lineage finalization.
		if runErr != nil || result.ExitCode != 0 {
			slog.Warn("failed to finalize job outputs after non-zero execution",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", finalizeErr)
			return runErr
		}
		return fmt.Errorf("finalize job outputs: %w", finalizeErr)
	}
	return runErr
}

func (r *runController) startRuntimeOutputSyncLoop(
	ctx context.Context,
	req StartRunRequest,
	cfg standardJobConfig,
	outDir, workspace string,
) func() {
	if cfg.RuntimeSync == nil {
		return func() {}
	}

	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if err := cfg.RuntimeSync(outDir, workspace); err != nil {
					slog.Warn("runtime output sync failed",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"error", err)
				}
			}
		}
	}()

	return func() {
		close(stop)
		<-done
		if err := cfg.RuntimeSync(outDir, workspace); err != nil {
			slog.Warn("runtime output sync final pass failed",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", err)
		}
	}
}

func normalizeBundlePath(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(filepath.ToSlash(n), "/"))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

// withTempDir creates a temporary directory, calls fn, then removes the directory.
func withTempDir(prefix string, fn func(path string) error) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return fmt.Errorf("create temp dir %s: %w", prefix, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("failed to remove temp dir", "path", dir, "error", err)
		}
	}()

	return fn(dir)
}

// withMaterializedResources materializes Hydra resources (In/Out/Home) from the
// manifest into a staging directory and passes the staging path to fn. When the
// manifest has no Hydra entries, fn receives "".
func (r *runController) withMaterializedResources(ctx context.Context, manifest contracts.StepManifest, bundleMap map[string]string, prefix string, fn func(stagingDir string) error) error {
	hashes := collectUniqueHashes(manifest)
	if len(hashes) == 0 {
		return fn("")
	}
	return withTempDir(prefix, func(dir string) error {
		if err := r.materializeHydraResources(ctx, manifest, bundleMap, dir); err != nil {
			return fmt.Errorf("materialize hydra resources: %w", err)
		}
		return fn(dir)
	})
}

// tempResource holds a temporary path and its cleanup function.
// Used for workspace snapshots, sticky workspaces, and similar lifecycle-scoped directories.
type tempResource struct {
	path    string
	cleanup func()
}

// prepareStickyWorkspaceWithCleanup wraps prepareStickyWorkspaceForStep and returns a no-op
// cleanup for sticky run/repo workspaces.
func (r *runController) prepareStickyWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (tempResource, error) {
	workspace, err := r.prepareStickyWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		return tempResource{}, err
	}

	return tempResource{
		path:    workspace,
		cleanup: func() {},
	}, nil
}

// clearManifestHydration removes hydration config from manifest inputs to prevent double-hydration.
func clearManifestHydration(manifest *contracts.StepManifest) {
	if len(manifest.Inputs) == 0 {
		return
	}
	inputs := make([]contracts.StepInput, len(manifest.Inputs))
	copy(inputs, manifest.Inputs)
	for i := range inputs {
		inputs[i].Hydration = nil
	}
	manifest.Inputs = inputs
}

// disableManifestGate sets Gate.Enabled=false on the manifest.
func disableManifestGate(manifest *contracts.StepManifest) {
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
}
