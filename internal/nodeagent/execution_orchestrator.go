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
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// executeRun orchestrates job execution based on job type.
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
		"job_type", req.JobType,
		"next_id", req.NextID,
	)

	jobType := req.JobType
	if jobType.IsZero() {
		jobType = inferJobTypeFromJobName(req.JobName)
		if !jobType.IsZero() {
			slog.Warn("job_type missing; inferred from job_name", "run_id", req.RunID, "job_id", req.JobID, "job_name", req.JobName, "job_type", jobType)
		}
	}

	// Dispatch based on job type from claim payload.
	switch jobType {
	case types.JobTypePreGate, types.JobTypePostGate, types.JobTypeReGate:
		req.JobType = jobType
		r.executeGateJob(ctx, req)
	case types.JobTypeMod:
		req.JobType = jobType
		r.executeModJob(ctx, req)
	case types.JobTypeHeal:
		req.JobType = jobType
		r.executeHealingJob(ctx, req)
	case types.JobTypeMR:
		req.JobType = jobType
		r.executeMRJob(ctx, req)
	default:
		err := fmt.Errorf("invalid job_type %q", jobType)
		slog.Error("cannot execute job with invalid type", "run_id", req.RunID, "job_id", req.JobID, "job_type", jobType, "error", err)
		r.uploadFailureStatus(ctx, req, err, 0)
	}
}

func inferJobTypeFromJobName(jobName string) types.JobType {
	name := strings.TrimSpace(jobName)
	switch {
	case strings.EqualFold(name, "pre-gate"):
		return types.JobTypePreGate
	case strings.EqualFold(name, "post-gate"):
		return types.JobTypePostGate
	case strings.HasPrefix(name, "re-gate-"):
		return types.JobTypeReGate
	case strings.HasPrefix(name, "heal-"):
		return types.JobTypeHeal
	case strings.HasPrefix(name, "mod-"):
		return types.JobTypeMod
	case strings.EqualFold(name, "mr"):
		return types.JobTypeMR
	default:
		return ""
	}
}

// modStepIndexFromJobName derives the mod step index from server-created job names.
// Expected shape is "mod-N". For single-step runs without an indexed name, returns 0.
func modStepIndexFromJobName(jobName string, stepsLen int) (int, error) {
	name := strings.TrimSpace(jobName)
	if stepsLen <= 1 {
		return 0, nil
	}

	if !strings.HasPrefix(name, "mod-") {
		return 0, fmt.Errorf("mod job_name must start with mod- for multi-step runs, got %q", name)
	}
	raw := strings.TrimPrefix(name, "mod-")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse mod index from job_name %q: %w", name, err)
	}
	if idx < 0 || idx >= stepsLen {
		return 0, fmt.Errorf("mod index out of range for job_name %q: idx=%d steps_len=%d", name, idx, stepsLen)
	}
	return idx, nil
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
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.JobID); uploadErr != nil {
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
