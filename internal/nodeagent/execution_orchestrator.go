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
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid step_index %v", f)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("step_index %v is not an integer", f)
	}
	if f < jobStepIndexModStart {
		return 0, fmt.Errorf("step_index %v is too small for a mod step", f)
	}

	// Server job layout (internal contract):
	// - pre-gate=1000
	// - mod-N=2000+N*1000
	// - post-gate=2000+(len(steps))*1000
	thousands := int64(f) / jobStepIndexInterval
	if int64(f) != thousands*jobStepIndexInterval {
		return 0, fmt.Errorf("step_index %v is not a multiple of 1000", f)
	}
	return int(thousands) - 2, nil
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
		StartTime:              startTime,
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
