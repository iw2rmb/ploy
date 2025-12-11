package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// maybeCompleteMultiStepRun checks if all jobs of a multi-step run are complete
// and transitions the run to its terminal state (succeeded/failed/canceled).
// This function derives the run's terminal status from the collective state of
// all jobs in a gate-aware way—the final gate result determines success/failure
// semantics for healing flows.
//
// Gate-aware status derivation rules:
//   - Fetch all jobs once and parse metadata to identify gate jobs (pre_gate, post_gate, re_gate).
//   - Track:
//   - hasNonGateFailure: whether any non-gate job (mod, heal) failed or was canceled.
//   - lastGateStatus: terminal status of the gate with the highest step_index.
//   - hasCanceled: whether any job was canceled (without failure precedence).
//   - Determine run status:
//   - If hasNonGateFailure: RunStatusFailed (mod/heal failures trump gate outcomes).
//   - Else if lastGateStatus == JobStatusFailed: RunStatusFailed (final gate failed).
//   - Else if hasCanceled: RunStatusCanceled.
//   - Else: RunStatusSucceeded.
//
// This avoids rewriting per-job terminal states after completion; each job's
// terminal status is set atomically by UpdateJobCompletion and remains unchanged.
func maybeCompleteMultiStepRun(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID domaintypes.RunID) error {
	// If the run is already in a terminal state, skip recomputation.
	if run.Status == store.RunStatusSucceeded || run.Status == store.RunStatusFailed || run.Status == store.RunStatusCanceled {
		return nil
	}
	// Fetch all jobs for the run to compute gate-aware status in a single pass.
	// runID is now a KSUID-backed string after run ID migration.
	jobs, err := st.ListJobsByRun(ctx, runID.String())
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	// Every run must have jobs. If there are no jobs, something is wrong.
	if len(jobs) == 0 {
		return fmt.Errorf("run has no jobs")
	}

	// Iterate through jobs (excluding MR jobs) to compute:
	// - terminalJobs: count of jobs in terminal state (for completion check).
	// - hasNonGateFailure: any non-gate job (mod/heal) failed or canceled.
	// - lastGateStepIndex + lastGateStatus: terminal status of highest-index gate.
	// - hasCanceled: any job was canceled (for fallback precedence).
	var (
		terminalJobs      int64
		totalJobs         int64
		hasNonGateFailure bool
		lastGateStepIndex float64
		lastGateStatus    store.JobStatus
		lastGateFound     bool
		hasCanceled       bool
	)

	for _, job := range jobs {
		modType := strings.TrimSpace(job.ModType)
		if modType == "mr" {
			// MR jobs are auxiliary and do not participate in run completion
			// status derivation. They must not block terminal state detection
			// or change success/failure semantics for the run.
			continue
		}

		totalJobs++

		// Check if job is in terminal state.
		isTerminal := job.Status == store.JobStatusSucceeded ||
			job.Status == store.JobStatusFailed ||
			job.Status == store.JobStatusCanceled ||
			job.Status == store.JobStatusSkipped
		if isTerminal {
			terminalJobs++
		}

		// Track canceled jobs for fallback precedence.
		if job.Status == store.JobStatusCanceled {
			hasCanceled = true
		}

		// Determine if this is a gate job based on mod_type column.
		isGate := modType == "pre_gate" || modType == "post_gate" || modType == "re_gate"

		if isGate {
			// Track the gate with the highest step_index (final gate result wins).
			if !lastGateFound || job.StepIndex > lastGateStepIndex {
				lastGateStepIndex = job.StepIndex
				lastGateStatus = job.Status
				lastGateFound = true
			}
			continue
		}

		// Non-gate jobs (mods, heal): check for failure/cancellation.
		// Non-gate failures take precedence over gate outcomes.
		if job.Status == store.JobStatusFailed || job.Status == store.JobStatusCanceled {
			hasNonGateFailure = true
		}
	}

	// If not all jobs are in terminal state, the run is still in progress.
	if terminalJobs < totalJobs {
		slog.Debug("multi-step run still in progress",
			"run_id", runID,
			"total_jobs", len(jobs),
			"terminal_jobs", terminalJobs,
		)
		return nil
	}

	// All jobs are in terminal state. Derive the run's terminal status using
	// gate-aware logic:
	// 1. Non-gate failures (mod/heal) trump everything → failed.
	// 2. Final gate failure → failed.
	// 3. Any cancellation (no failures) → canceled.
	// 4. All succeeded → succeeded.
	var runStatus store.RunStatus
	switch {
	case hasNonGateFailure:
		// Mod/heal job failed or was canceled → run failed.
		runStatus = store.RunStatusFailed
	case lastGateFound && lastGateStatus == store.JobStatusFailed:
		// Final gate failed (healing didn't recover) → run failed.
		runStatus = store.RunStatusFailed
	case hasCanceled:
		// Some job was canceled but no failures → run canceled.
		runStatus = store.RunStatusCanceled
	default:
		// All jobs succeeded (including final gate) → run succeeded.
		runStatus = store.RunStatusSucceeded
	}

	slog.Info("multi-step run completing",
		"run_id", runID,
		"total_jobs", len(jobs),
		"terminal_jobs", terminalJobs,
		"derived_status", runStatus,
		"last_gate_status", lastGateStatus,
		"has_non_gate_failure", hasNonGateFailure,
	)

	// Transition the run to its terminal status.
	// Use empty JSON object for stats (step-level stats are tracked per step).
	// Note: We intentionally do NOT mutate per-job terminal states here—each job's
	// status was set atomically by UpdateJobCompletion and should remain unchanged.
	err = st.UpdateRunCompletion(ctx, store.UpdateRunCompletionParams{
		ID:     runID.String(),
		Status: runStatus,
		Stats:  []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("update run completion: %w", err)
	}

	// Publish terminal run summary event and done status to the SSE hub.
	if eventsService != nil {
		// Map store.RunStatus to modsapi.RunState.
		var runState modsapi.RunState
		switch runStatus {
		case store.RunStatusSucceeded:
			runState = modsapi.RunStateSucceeded
		case store.RunStatusFailed:
			runState = modsapi.RunStateFailed
		case store.RunStatusCanceled:
			runState = modsapi.RunStateCancelled
		default:
			runState = modsapi.RunStateFailed
		}

		// Publish run summary event with final run state.
		summary := modsapi.RunSummary{
			RunID:      runID,
			State:      runState,
			Repository: run.RepoUrl,
			CreatedAt:  run.CreatedAt.Time,
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}
		if err := eventsService.PublishRun(ctx, runID, summary); err != nil {
			slog.Error("complete multi-step run: publish run summary event failed", "run_id", runID, "err", err)
		}

		// Publish done event to signal stream completion.
		doneStatus := logstream.Status{Status: "done"}
		if err := eventsService.Hub().PublishStatus(ctx, runID.String(), doneStatus); err != nil {
			slog.Error("complete multi-step run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("multi-step run completed",
		"run_id", runID,
		"status", runStatus,
	)

	// If this run is a child execution run linked to a RunRepo, update the repo's status.
	// This connects the batch orchestration layer to per-repo execution outcomes.
	if err := maybeUpdateRunRepoFromExecution(ctx, st, runID, runStatus); err != nil {
		// Log but don't fail — the run completion itself succeeded.
		slog.Warn("multi-step run completed but failed to update linked run_repo",
			"run_id", runID,
			"status", runStatus,
			"err", err,
		)
	}

	// After the run reaches a terminal state, schedule a best-effort MR job
	// when MR wiring is configured. MR jobs run after completion and must
	// not influence the run's terminal status.
	if err := maybeScheduleMRJobForRun(ctx, st, run, runID, runStatus); err != nil {
		slog.Error("multi-step run completed but failed to schedule MR job",
			"run_id", runID,
			"status", runStatus,
			"err", err,
		)
	}

	return nil
}

// maybeUpdateRunRepoFromExecution checks if the completed run is linked to a RunRepo entry
// (i.e., it's a child execution run created by the batch orchestrator) and updates the
// repo's status to match the execution outcome. This enables batch-level status aggregation.
//
// RunStatus → RunRepoStatus mapping:
//   - succeeded → succeeded
//   - failed    → failed
//   - canceled  → cancelled (note spelling difference)
func maybeUpdateRunRepoFromExecution(ctx context.Context, st store.Store, runID domaintypes.RunID, runStatus store.RunStatus) error {
	// Look up the RunRepo entry that references this run as its execution_run_id.
	// runID is now a KSUID string after run ID migration.
	runIDStr := runID.String()
	runRepo, err := st.GetRunRepoByExecutionRun(ctx, &runIDStr)
	if err != nil {
		// If no RunRepo is linked to this run, it's a standalone run (not part of a batch).
		// This is expected for single-repo runs created via /v1/mods; silently skip.
		if err.Error() == "no rows in result set" {
			return nil
		}
		return fmt.Errorf("get run_repo by execution_run: %w", err)
	}

	// Map RunStatus to RunRepoStatus.
	var repoStatus store.RunRepoStatus
	switch runStatus {
	case store.RunStatusSucceeded:
		repoStatus = store.RunRepoStatusSucceeded
	case store.RunStatusFailed:
		repoStatus = store.RunRepoStatusFailed
	case store.RunStatusCanceled:
		repoStatus = store.RunRepoStatusCancelled // Note: RunRepoStatus uses British spelling.
	default:
		// Unexpected status; default to failed for safety.
		repoStatus = store.RunRepoStatusFailed
		slog.Warn("unexpected run status when updating run_repo",
			"run_id", runID,
			"run_status", runStatus,
		)
	}

	// Update the RunRepo status to reflect the execution outcome.
	// runRepo.ID is now a NanoID string.
	err = st.UpdateRunRepoStatus(ctx, store.UpdateRunRepoStatusParams{
		ID:     runRepo.ID,
		Status: repoStatus,
	})
	if err != nil {
		return fmt.Errorf("update run_repo status: %w", err)
	}

	slog.Info("run_repo status updated from execution run",
		"run_repo_id", runRepo.ID, // RunRepo IDs are NanoID strings.
		"run_id", runID, // Run IDs are KSUID strings.
		"batch_run_id", runRepo.RunID,
		"status", repoStatus,
	)

	return nil
}
