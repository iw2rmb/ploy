package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// Resumability invariants:
// 1. Only terminal states (failed, canceled) are resumable.
// 2. Succeeded runs cannot be resumed — there is nothing to fix.
// 3. In-progress runs (queued, assigned, running) return 200 OK for idempotency.
// 4. Already-succeeded jobs within a run are preserved; only failed/canceled jobs are reset.
// 5. A job that is already pending/running triggers idempotent 200 OK (no double-scheduling).

// resumeTicketHandler resumes a failed or canceled Mods  run (run) by requeueing eligible jobs.
// POST /v1/mods/{id}/resume
// Responses:
//   - 202 Accepted on successful resume initiation
//   - 200 OK if already running/queued (idempotent) or if all jobs already succeeded
//   - 404 Not Found if  run does not exist
//   - 400 Bad Request for invalid id
//   - 409 Conflict if the run cannot be resumed (e.g., state=succeeded is not resumable)
func resumeTicketHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and validate the  run ID from the path.
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse UUID using domain types helper for consistent validation.
		pgID := domaintypes.ToPGUUID(runIDStr)
		if !pgID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Fetch current run state from database.
		run, err := st.GetRun(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, " run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get  run: %v", err), http.StatusInternalServerError)
			slog.Error("resume  run: lookup failed", " run_id", runIDStr, "err", err)
			return
		}

		// Check resumability invariants using a centralized helper.
		// This ensures consistent error messages and logging across resume paths.
		resumable, httpStatus, errMsg := checkResumability(run)
		if !resumable {
			// Log rejected resume attempts for observability.
			slog.Info("resume rejected", " run_id", runIDStr, "state", run.Status, "reason", errMsg)
			if httpStatus == http.StatusOK {
				// Idempotent case: run is already in progress.
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Error(w, errMsg, httpStatus)
			return
		}

		// Fetch all jobs for this run to determine which need to be requeued.
		jobs, err := st.ListJobsByRun(r.Context(), pgID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			slog.Error("resume  run: list jobs failed", " run_id", runIDStr, "err", err)
			return
		}

		// Identify jobs that need to be reset and find the first job to schedule.
		// Jobs in terminal failure states (failed, canceled) should be reset to 'created'.
		// Jobs already succeeded or skipped remain untouched.
		// The first non-succeeded job in order (by step_index) will be set to 'pending'.
		var firstJobToSchedule *store.Job
		jobsToReset := make([]store.Job, 0)

		for i := range jobs {
			job := &jobs[i]
			switch job.Status {
			case store.JobStatusSucceeded, store.JobStatusSkipped:
				// Already complete, leave unchanged.
				continue
			case store.JobStatusFailed, store.JobStatusCanceled:
				// Terminal failure state - needs reset to 'created' so it can be rescheduled.
				jobsToReset = append(jobsToReset, *job)
				if firstJobToSchedule == nil {
					firstJobToSchedule = job
				}
			case store.JobStatusCreated, store.JobStatusPending, store.JobStatusRunning:
				// Jobs still in progress (shouldn't happen if run is terminal, but handle defensively).
				// If we encounter a pending/running job, resume is idempotent.
				if job.Status == store.JobStatusPending || job.Status == store.JobStatusRunning {
					// Run has active jobs already; no need to resume.
					w.WriteHeader(http.StatusOK)
					return
				}
				// 'created' job should be scheduled.
				if firstJobToSchedule == nil {
					firstJobToSchedule = job
				}
			}
		}

		// If all jobs succeeded (nothing to reset), the run should have been succeeded.
		// Return 200 OK as idempotent - nothing to do.
		if len(jobsToReset) == 0 && firstJobToSchedule == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Reset failed/canceled jobs back to 'created' status (except the first one, which goes to 'pending').
		// Clear timing fields to allow fresh execution.
		for _, job := range jobsToReset {
			newStatus := store.JobStatusCreated
			// The first job to schedule should go directly to 'pending' so it can be claimed.
			if firstJobToSchedule != nil && job.ID == firstJobToSchedule.ID {
				newStatus = store.JobStatusPending
			}

			err := st.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
				ID:         job.ID,
				Status:     newStatus,
				StartedAt:  pgtype.Timestamptz{Valid: false}, // Clear start time.
				FinishedAt: pgtype.Timestamptz{Valid: false}, // Clear finish time.
				DurationMs: 0,                                // Clear duration.
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to reset job: %v", err), http.StatusInternalServerError)
				slog.Error("resume  run: update job status failed", " run_id", runIDStr, "job_id", job.ID, "err", err)
				return
			}
		}

		// If no jobs were reset but we have a firstJobToSchedule (must be 'created' status),
		// transition it to 'pending' using ScheduleNextJob for consistency.
		if len(jobsToReset) == 0 && firstJobToSchedule != nil && firstJobToSchedule.Status == store.JobStatusCreated {
			if _, err := st.ScheduleNextJob(r.Context(), pgID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				slog.Error("resume  run: schedule next job failed", " run_id", runIDStr, "err", err)
				// Non-fatal: job is already 'created' and will be picked up by future scheduling.
			}
		}

		// Transition run back to 'queued' status to indicate it's ready for execution.
		// Clear finished_at since the run is no longer terminal.
		err = st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{
			ID:         pgID,
			Status:     store.RunStatusQueued,
			FinishedAt: pgtype.Timestamptz{Valid: false}, // Clear terminal timestamp.
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to resume  run: %v", err), http.StatusInternalServerError)
			slog.Error("resume  run: update run status failed", " run_id", runIDStr, "err", err)
			return
		}

		// Track resume metadata (resume_count, last_resumed_at) in runs.stats.
		// This allows clients to see resume history via  run status and SSE events.
		if err := st.UpdateRunResume(r.Context(), pgID); err != nil {
			// Log but don't fail the operation; the resume itself succeeded.
			slog.Error("resume  run: update resume stats failed", " run_id", runIDStr, "err", err)
		}

		// Publish  run event for SSE clients to indicate the run has resumed.
		// Include resume metadata so watchers can see the resume transition in the stream.
		if eventsService != nil {
			now := time.Now().UTC()
			runSummary := modsapi.RunSummary{
				TicketID:   domaintypes.TicketID(uuid.UUID(pgID.Bytes).String()),
				State:      modsapi.TicketStatePending, // 'pending' maps to 'queued' in mods API.
				Repository: run.RepoUrl,
				Metadata:   map[string]string{"repo_base_ref": run.BaseRef, "repo_target_ref": run.TargetRef},
				CreatedAt:  timeOrZero(run.CreatedAt),
				UpdatedAt:  now,
				Stages:     make(map[string]modsapi.StageStatus),
			}
			// Re-fetch run to get updated stats with resume_count and last_resumed_at.
			// Use fresh stats to ensure event reflects the latest resume metadata.
			if updatedRun, err := st.GetRun(r.Context(), pgID); err == nil {
				runSummary.Metadata = buildResumeMetadata(updatedRun)
			}
			if err := eventsService.PublishTicket(r.Context(), uuid.UUID(pgID.Bytes).String(), runSummary); err != nil {
				slog.Error("resume  run: publish  run event failed", " run_id", runIDStr, "err", err)
			}
		}

		w.WriteHeader(http.StatusAccepted)
		slog.Info(" run resumed", " run_id", runIDStr, "jobs_reset", len(jobsToReset))
	}
}

// checkResumability evaluates whether a run can be resumed based on its current state.
// It returns (resumable, httpStatus, errorMessage):
//   - resumable=true: the run can proceed with resume logic
//   - resumable=false with httpStatus=200: idempotent case (already in progress)
//   - resumable=false with httpStatus=409: conflict (state not resumable)
//
// Error messages follow the format: " run state=<state> is not resumable[: reason]"
// to provide clear, consistent feedback to API clients.
func checkResumability(run store.Run) (resumable bool, httpStatus int, errMsg string) {
	switch run.Status {
	case store.RunStatusQueued, store.RunStatusAssigned, store.RunStatusRunning:
		// Invariant 3: In-progress runs return 200 OK for idempotency.
		// The run is already active; no action needed.
		return false, http.StatusOK, fmt.Sprintf(" run state=%s is already in progress", run.Status)

	case store.RunStatusSucceeded:
		// Invariant 2: Succeeded runs cannot be resumed — nothing to fix.
		return false, http.StatusConflict, fmt.Sprintf(" run state=%s is not resumable: nothing to fix", run.Status)

	case store.RunStatusFailed, store.RunStatusCanceled:
		// Invariant 1: Terminal failure states are resumable.
		return true, 0, ""

	default:
		// Unknown state: reject with 409 to be safe.
		return false, http.StatusConflict, fmt.Sprintf(" run state=%s is not resumable", run.Status)
	}
}

// buildResumeMetadata extracts standard  run metadata from a run, including
// resume-related fields (resume_count, last_resumed_at) when present in runs.stats.
// Used by resume handler to populate RunSummary.Metadata for SSE events.
func buildResumeMetadata(run store.Run) map[string]string {
	meta := map[string]string{
		"repo_base_ref":   run.BaseRef,
		"repo_target_ref": run.TargetRef,
	}
	// Include claiming node id when available for diagnostics.
	if run.NodeID.Valid {
		meta["node_id"] = uuid.UUID(run.NodeID.Bytes).String()
	}
	// Parse stats to extract resume metadata.
	if len(run.Stats) > 0 && json.Valid(run.Stats) {
		var stats domaintypes.RunStats
		if err := json.Unmarshal(run.Stats, &stats); err == nil {
			// Add resume_count if run has been resumed at least once.
			if rc := stats.ResumeCount(); rc > 0 {
				meta["resume_count"] = strconv.Itoa(rc)
			}
			// Add last_resumed_at timestamp when available.
			if lra := stats.LastResumedAt(); lra != "" {
				meta["last_resumed_at"] = lra
			}
		}
	}
	return meta
}
