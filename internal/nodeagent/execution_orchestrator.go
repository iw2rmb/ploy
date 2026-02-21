// execution_orchestrator.go contains the high-level run lifecycle orchestration.
//
// This file owns executeRun, the main entry point for executing a single run.
// It coordinates workspace setup, runtime initialization, healing execution,
// artifact collection, and terminal status reporting. The orchestrator ensures
// cleanup of ephemeral resources and delegates domain-specific concerns to
// specialized execution files (healing, MR creation, uploads).
package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

const (
	// Job layout constants (internal contract)
	jobStepIndexModStart = 2000
	jobStepIndexInterval = 1000
)

// executeRun orchestrates job execution based on job type (ModType).
// Dispatches to specialized handlers: gate jobs, mod jobs, or healing jobs.
//
// Job types:
//   - pre_gate, post_gate, re_gate: Run build gate validation
//   - mod: Run container with mod execution
//   - heal: Run healing container after gate failure
//
// Each job is atomic - there's no multi-step loop. The server creates
// individual jobs (pre-gate, mod-0, ..., post-gate) and nodes execute
// them independently. Healing jobs are created by the server when
// gates fail, not run inline by the node.
func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	defer func() {
		// Recover from panics to prevent job leaks and slot exhaustion.
		// Log the panic and stack trace for debugging.
		if p := recover(); p != nil {
			slog.Error("executeRun panic recovered",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"panic", p,
				"stack", string(debug.Stack()),
			)
		}

		r.mu.Lock()
		// Use typed JobID directly as map key — no string conversion needed.
		delete(r.jobs, req.JobID)
		r.mu.Unlock()

		// Release the concurrency slot acquired in claimAndExecute.
		// This frees the slot for the next job to be claimed.
		r.ReleaseSlot()
	}()

	slog.Info("starting job execution",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"mod_type", req.ModType,
		"step_index", req.StepIndex,
	)

	// Dispatch based on job type (ModType).
	switch req.ModType {
	case types.ModTypePreGate, types.ModTypePostGate, types.ModTypeReGate:
		r.executeGateJob(ctx, req)
	case types.ModTypeMod:
		r.executeModJob(ctx, req)
	case types.ModTypeHeal:
		r.executeHealingJob(ctx, req)
	case types.ModTypeMR:
		r.executeMRJob(ctx, req)
	default:
		// Fallback for legacy jobs without ModType - execute as mod job.
		slog.Warn("unknown mod_type, falling back to mod execution",
			"run_id", req.RunID,
			"mod_type", req.ModType,
		)
		r.executeModJob(ctx, req)
	}
}

// executeModJob runs a mod container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mod steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeModJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	typedOpts := req.TypedOptions
	stepIdx := 0
	if len(typedOpts.Steps) > 0 {
		idx, err := modStepIndexFromJobStepIndex(req.StepIndex)
		if err != nil {
			err = fmt.Errorf("derive mod step index from step_index: %w", err)
			slog.Error("failed to derive mod step index", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		if idx < 0 || idx >= len(typedOpts.Steps) {
			err := fmt.Errorf("derived mod step index out of range: step_index=%v derived=%d steps_len=%d", req.StepIndex, idx, len(typedOpts.Steps))
			slog.Error("derived mod step index out of range", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "derived_index", idx, "steps_len", len(typedOpts.Steps))
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		stepIdx = idx
	}
	manifest, err := buildManifestFromRequest(req, typedOpts, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mod job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest:                  manifest,
		DiffType:                  DiffModTypeMod,
		OutDirPattern:             "ploy-mod-out-*",
		UploadConfiguredArtifacts: true,
		UploadDiff:                r.uploadModDiffWithBaseline,
		StartTime:                 startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

func modStepIndexFromJobStepIndex(stepIndex types.StepIndex) (int, error) {
	f := stepIndex.Float64()

	// Validate: must be a finite integer >= jobStepIndexModStart and multiple of 1000.
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, fmt.Errorf("step_index %v is not a valid integer", f)
	}

	idx := int64(f)
	if idx < jobStepIndexModStart || idx%jobStepIndexInterval != 0 {
		return 0, fmt.Errorf("step_index %v is not a valid mod step (must be >= %d and multiple of %d)",
			f, jobStepIndexModStart, jobStepIndexInterval)
	}

	// Server job layout: mod-N = 2000 + N*1000, so N = (idx/1000) - 2
	return int(idx/jobStepIndexInterval) - 2, nil
}

// executeHealingJob runs a healing container job.
// Fetches gate logs from parent job, runs healing container, uploads diff.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures healing
// mods use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeHealingJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	var manifest contracts.StepManifest
	var err error

	// When build_gate_healing is configured, hydrate the healing manifest from the
	// typed HealingConfig so that discrete healing jobs use the correct image/env.
	if typedOpts.Healing != nil && !typedOpts.Healing.Mod.Image.IsEmpty() {
		healMod := typedOpts.Healing.Mod
		manifest, err = buildHealingManifest(req, healMod, 0, "", stack)
	}
	if manifest.Image == "" {
		manifest, err = buildManifestFromRequest(req, typedOpts, 0, stack)
	}
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("healing job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      DiffModTypeHealing,
		OutDirPattern: "ploy-heal-out-*",
		InDirPattern:  "ploy-heal-in-*",
		PopulateInDir: func(inDir string) error {
			return r.populateHealingInDir(req.RunID, inDir)
		},
		InjectEnv: func(m *contracts.StepManifest, ws string) {
			r.injectHealingEnvVars(m, ws)
		},
		MountCerts: func(m *contracts.StepManifest) {
			r.mountHealingTLSCerts(m)
		},
		CheckWorkspaceNoChange: true,
		UploadDiff:             r.uploadHealingJobDiff,
		BuildJobMeta: func(outDir string) json.RawMessage {
			actionSummary := parseActionSummary(outDir)
			if actionSummary == "" {
				return nil
			}
			meta := &contracts.JobMeta{
				Kind:          contracts.JobKindMod,
				ActionSummary: actionSummary,
			}
			data, err := contracts.MarshalJobMeta(meta)
			if err != nil {
				slog.Warn("failed to marshal healing job meta", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				return nil
			}
			return data
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// uploadHealingNoWorkspaceChangesFailure uploads a terminal failure status when a healing job
// exits 0 but produces no workspace changes.
func (r *runController) uploadHealingNoWorkspaceChangesFailure(ctx context.Context, req StartRunRequest, baseStats types.RunStats, duration time.Duration) {
	// This is considered a failure: the healing mod promised to fix the issue but
	// didn't actually change anything. Upload a failed status with exit code 1 and
	// a stable stats marker so downstream observers can distinguish this from other
	// failure modes.
	//
	// Since RunStats is now json.RawMessage-backed, we build a new stats object
	// with the healing_warning field included.
	stats := types.NewRunStatsBuilder().
		ExitCode(1).
		DurationMs(duration.Milliseconds()).
		HealingWarning("no_workspace_changes").
		MustBuild()

	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	var exitCodeOne int32 = 1
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCodeOne, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing failure status (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("healing job failed (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "exit_code", 1, "duration", duration)
}

// populateHealingInDir copies the first failing gate log (when present) into
// the healing job's /in directory as build-gate.log. This mirrors the behavior
// of executeWithHealing, which writes a trimmed failure view for Codex healers.
func (r *runController) populateHealingInDir(runID types.RunID, inDir string) error {
	if strings.TrimSpace(inDir) == "" {
		return nil
	}

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	srcPath := filepath.Join(runDir, "build-gate-first.log")

	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read first gate log: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	destPath := filepath.Join(inDir, "build-gate.log")
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("write /in/build-gate.log: %w", err)
	}

	slog.Info("hydrated /in/build-gate.log for healing job", "run_id", runID, "path", destPath)
	return nil
}

// uploadFailureStatus uploads a failure status for early errors.
// Uses exit code -1 to indicate pre-execution infrastructure failures.
// v1 uses capitalized job status values: Success, Fail, Cancelled.
func (r *runController) uploadFailureStatus(ctx context.Context, req StartRunRequest, err error, duration time.Duration) {
	status := JobStatusFail
	var exitCode *int32
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		status = JobStatusCancelled
	} else {
		var preExecutionExitCode int32 = -1 // -1 indicates pre-execution failure
		exitCode = &preExecutionExitCode
	}

	// Build stats using typed builder to eliminate map[string]any construction.
	stats := types.NewRunStatsBuilder().
		DurationMs(duration.Milliseconds()).
		Error(err.Error()).
		MustBuild()
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
}

// initializeRuntime creates and configures all runtime components needed for step execution.
// Returns a configured step.Runner, diff generator, and log streamer.
//
// Parameters:
//   - ctx: context for initialization operations
//   - runID: run identifier for logging and telemetry
//   - jobID: job identifier for associating log chunks with specific jobs; pass a zero value
//     only when job attribution is not available
func (r *runController) initializeRuntime(ctx context.Context, runID types.RunID, jobID types.JobID) (step.Runner, step.DiffGenerator, *LogStreamer, error) {
	// Initialize git fetcher without snapshot publishing (node agent operates on ephemeral workspaces).
	gitFetcher, err := r.createGitFetcher()
	if err != nil {
		return step.Runner{}, nil, nil, fmt.Errorf("create git fetcher: %w", err)
	}

	// Initialize workspace hydrator with git fetcher.
	workspaceHydrator, err := r.createWorkspaceHydrator(gitFetcher)
	if err != nil {
		return step.Runner{}, nil, nil, fmt.Errorf("create workspace hydrator: %w", err)
	}

	// Initialize container runtime with image pull enabled.
	// Fallback to nil if Docker is unavailable (simulated execution mode).
	containerRuntime, err := r.createContainerRuntime()
	if err != nil {
		slog.Warn("docker unavailable; falling back to stub runtime", "run_id", runID, "error", err)
		containerRuntime = nil
	}

	// Initialize diff generator for workspace change detection.
	diffGenerator := r.createDiffGenerator()

	// Initialize gate executor using local Docker-based execution.
	// All gates run via the container runtime.
	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

	// Initialize log streamer to stream logs as gzipped chunks to the server.
	// The jobID parameter associates log chunks with a specific job, enabling
	// per-job log attribution in the control plane.
	logStreamer, err := NewLogStreamer(r.cfg, runID, jobID)
	if err != nil {
		return step.Runner{}, nil, nil, fmt.Errorf("create log streamer: %w", err)
	}

	// Assemble the step runner with all components.
	runner := step.Runner{
		Workspace:  workspaceHydrator,
		Containers: containerRuntime,
		Diffs:      diffGenerator,
		Gate:       gateExecutor,
		LogWriter:  logStreamer,
	}

	return runner, diffGenerator, logStreamer, nil
}

// mergeExecutionResults aggregates gate history across phases (pre-mod + per-step)
// while keeping the latest step.Result for terminal status reporting.
// - PreGate is preserved from the accumulator when present (pre-mod gate).
// - ReGates are appended in call order to accumulate healing re-gates.
func mergeExecutionResults(acc executionResult, next executionResult) executionResult {
	merged := executionResult{
		Result:  next.Result,
		PreGate: acc.PreGate,
		ReGates: acc.ReGates,
	}

	// If there is no pre-mod gate recorded yet, fall back to the next result's PreGate.
	if merged.PreGate == nil && next.PreGate != nil {
		merged.PreGate = next.PreGate
	}

	// Append any re-gates from the next execution in order.
	if len(next.ReGates) > 0 {
		merged.ReGates = append(merged.ReGates, next.ReGates...)
	}

	return merged
}

// jobExecutionContext holds runtime components initialized for a mod/heal job.
type jobExecutionContext struct {
	runner        step.Runner
	diffGenerator step.DiffGenerator
	logStreamer   *LogStreamer
}

// initJobExecutionContext initializes runtime components and returns a cleanup function
// that closes the logStreamer (must be deferred).
func (r *runController) initJobExecutionContext(ctx context.Context, runID types.RunID, jobID types.JobID) (jobExecutionContext, func(), error) {
	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, runID, jobID)
	if err != nil {
		return jobExecutionContext{}, nil, err
	}

	execCtx := jobExecutionContext{
		runner:        runner,
		diffGenerator: diffGenerator,
		logStreamer:   logStreamer,
	}

	cleanup := func() {
		if err := logStreamer.Close(); err != nil {
			slog.Warn("failed to close log streamer", "run_id", runID, "job_id", jobID, "error", err)
		}
	}

	return execCtx, cleanup, nil
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

// snapshotResult holds a workspace snapshot path and its cleanup function.
type snapshotResult struct {
	dir     string
	cleanup func()
}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
func snapshotWorkspaceForNoIndexDiff(runID types.RunID, jobID types.JobID, diffType DiffModType, workspace string) snapshotResult {
	jobTypeStr := diffType.String()
	prefix := fmt.Sprintf("ploy-%s-base-*", jobTypeStr)
	snapshotDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to create baseline snapshot directory", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	if err := copyGitClone(workspace, snapshotDir); err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to snapshot baseline workspace", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
			slog.Warn("failed to remove snapshot dir after copy failure", "path", snapshotDir, "error", rmErr)
		}
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	return snapshotResult{
		dir: snapshotDir,
		cleanup: func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove snapshot dir", "path", snapshotDir, "error", err)
			}
		},
	}
}

// workspaceRehydrationResult holds a rehydrated workspace path and its cleanup function.
type workspaceRehydrationResult struct {
	workspace string
	cleanup   func()
}

// rehydrateWorkspaceWithCleanup wraps rehydrateWorkspaceForStep with automatic cleanup.
func (r *runController) rehydrateWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	stepIndex types.StepIndex,
) (workspaceRehydrationResult, error) {
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, stepIndex)
	if err != nil {
		return workspaceRehydrationResult{}, err
	}

	return workspaceRehydrationResult{
		workspace: workspace,
		cleanup: func() {
			if err := os.RemoveAll(workspace); err != nil {
				slog.Warn("failed to remove workspace", "path", workspace, "error", err)
			}
		},
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

// standardJobConfig configures the execution of a standard container job (mod/heal).
type standardJobConfig struct {
	Manifest      contracts.StepManifest
	DiffType      DiffModType
	OutDirPattern string
	InDirPattern  string

	PopulateInDir func(inDir string) error
	InjectEnv     func(manifest *contracts.StepManifest, workspace string)
	MountCerts    func(manifest *contracts.StepManifest)

	CheckWorkspaceNoChange    bool
	UploadConfiguredArtifacts bool

	UploadDiff   func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baselineDir, workspace string, result step.Result, stepIndex types.StepIndex)
	BuildJobMeta func(outDir string) json.RawMessage

	StartTime time.Time
}

// executeStandardJob orchestrates the common lifecycle of a container job (mod/heal):
// runtime init, rehydration, snapshots, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, cfg.Manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.dir
	}

	executeBody := func(outDir, inDir string) error {
		manifest := cfg.Manifest
		disableManifestGate(&manifest)
		clearManifestHydration(&manifest)

		if cfg.InjectEnv != nil {
			cfg.InjectEnv(&manifest, workspace)
		}
		if cfg.MountCerts != nil {
			cfg.MountCerts(&manifest)
		}

		imageName := strings.TrimSpace(manifest.Image)
		if imageName == "" {
			return fmt.Errorf("resolved job image is empty")
		}
		if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
			return fmt.Errorf("save job image name: %w", err)
		}

		var preStatus string
		var preStatusErr error
		if cfg.CheckWorkspaceNoChange {
			preStatus, preStatusErr = workspaceStatus(ctx, workspace)
			if preStatusErr != nil {
				slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
			}
		}

		result, runErr := execCtx.runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			Manifest:  manifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     inDir,
		})
		duration := time.Since(startTime)

		if cfg.UploadDiff != nil {
			cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result, req.StepIndex)
		}

		if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
		}

		if cfg.UploadConfiguredArtifacts {
			r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace)
		}

		if cfg.CheckWorkspaceNoChange && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
			postStatus, postErr := workspaceStatus(ctx, workspace)
			if postErr == nil && postStatus == preStatus {
				r.uploadHealingNoWorkspaceChangesFailure(ctx, req, types.NewRunStatsBuilder().ExitCode(1).DurationMs(duration.Milliseconds()).HealingWarning("no_workspace_changes").MustBuild(), duration)
				return nil
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

		if cfg.BuildJobMeta != nil {
			if meta := cfg.BuildJobMeta(outDir); len(meta) > 0 {
				statsBuilder.JobMeta(meta)
			}
		}

		stats := statsBuilder.MustBuild()

		if runErr != nil {
			status := JobStatusFail
			var exitCode *int32
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				status = JobStatusCancelled
			} else {
				var runtimeExitCode int32 = -1
				exitCode = &runtimeExitCode
			}
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
			}
			slog.Info("job terminated", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "status", status, "duration", duration, "error", runErr)
			return nil
		}

		if result.ExitCode != 0 {
			exitCode := int32(result.ExitCode)
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload failure status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
			}
			slog.Info("job failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "exit_code", result.ExitCode, "duration", duration)
			return nil
		}

		var exitCodeZero int32 = 0
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusSuccess.String(), &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload success status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
		}
		slog.Info("job succeeded", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "duration", duration)
		return nil
	}

	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						slog.Warn("failed to populate in dir", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
					}
				}
				return executeBody(outDir, inDir)
			})
		}
		return executeBody(outDir, "")
	})

	if outDirErr != nil {
		slog.Error("failed to create temp directories", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}
