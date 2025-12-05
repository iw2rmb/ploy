package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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
func maybeCompleteMultiStepRun(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID pgtype.UUID) error {
	// Fetch all jobs for the run to compute gate-aware status in a single pass.
	jobs, err := st.ListJobsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	// Every run must have jobs. If there are no jobs, something is wrong.
	if len(jobs) == 0 {
		return fmt.Errorf("run has no jobs")
	}

	// Iterate through jobs to compute:
	// - terminalJobs: count of jobs in terminal state (for completion check).
	// - hasNonGateFailure: any non-gate job (mod/heal) failed or canceled.
	// - lastGateStepIndex + lastGateStatus: terminal status of highest-index gate.
	// - hasCanceled: any job was canceled (for fallback precedence).
	var (
		terminalJobs      int64
		hasNonGateFailure bool
		lastGateStepIndex float64
		lastGateStatus    store.JobStatus
		lastGateFound     bool
		hasCanceled       bool
	)

	for _, job := range jobs {
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
		modType := strings.TrimSpace(job.ModType)
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
	if terminalJobs < int64(len(jobs)) {
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
		ID:     runID,
		Status: runStatus,
		Stats:  []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("update run completion: %w", err)
	}

	// Publish terminal ticket event and done status to SSE hub.
	if eventsService != nil {
		// Map store.RunStatus to modsapi.TicketState.
		var ticketState modsapi.TicketState
		switch runStatus {
		case store.RunStatusSucceeded:
			ticketState = modsapi.TicketStateSucceeded
		case store.RunStatusFailed:
			ticketState = modsapi.TicketStateFailed
		case store.RunStatusCanceled:
			ticketState = modsapi.TicketStateCancelled
		default:
			ticketState = modsapi.TicketStateFailed
		}

		runUUID := uuid.UUID(runID.Bytes)
		ticketSummary := modsapi.TicketSummary{
			TicketID:   domaintypes.TicketID(runUUID.String()),
			State:      ticketState,
			Repository: run.RepoUrl,
			CreatedAt:  run.CreatedAt.Time,
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}
		if err := eventsService.PublishTicket(ctx, runUUID.String(), ticketSummary); err != nil {
			slog.Error("complete multi-step run: publish ticket event failed", "run_id", runID, "err", err)
		}

		// Publish done event to signal stream completion.
		doneStatus := logstream.Status{Status: "done"}
		if err := eventsService.Hub().PublishStatus(ctx, runUUID.String(), doneStatus); err != nil {
			slog.Error("complete multi-step run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("multi-step run completed",
		"run_id", runID,
		"status", runStatus,
	)

	return nil
}
