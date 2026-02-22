// execution_orchestrator.go contains the high-level run lifecycle orchestration.
//
// This file owns executeRun, the main entry point for executing a single run.
// It coordinates runtime initialization and dispatches to specialized job
// handlers based on job type. Job implementations live in:
//   - execution_orchestrator_jobs.go — mod and healing jobs + standard executor
//   - execution_orchestrator_gate.go — gate validation jobs
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
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
