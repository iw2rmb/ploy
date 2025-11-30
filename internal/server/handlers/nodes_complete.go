package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// completeRunHandler marks a run as completed with terminal status and stats.
// Sets finished_at timestamp and populates runs.stats field.
func completeRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeID := domaintypes.ToPGUUID(nodeIDStr)
		if !nodeID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id, status, exit_code, stats, and step_index.
		// Nodeagent includes step_index to identify which job is being completed.
		var req struct {
			RunID     domaintypes.RunID     `json:"run_id"`
			Status    string                `json:"status"`
			ExitCode  *int32                `json:"exit_code,omitempty"` // Exit code from job execution
			Stats     json.RawMessage       `json:"stats,omitempty"`
			StepIndex domaintypes.StepIndex `json:"step_index"` // Job step index for completion
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if req.RunID.IsZero() {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runID := domaintypes.ToPGUUID(req.RunID.String())
		if !runID.Valid {
			http.Error(w, "invalid run_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Validate and convert status to canonical RunStatus type.
		// This uses ConvertToRunStatus to handle various API representations
		// (e.g., "cancelled" vs "canceled", "pending" -> "queued").
		if strings.TrimSpace(req.Status) == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}

		normalizedStatus, err := store.ConvertToRunStatus(strings.ToLower(strings.TrimSpace(req.Status)))
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid status: %v", err), http.StatusBadRequest)
			return
		}

		// Validate that status is a terminal state (succeeded, failed, or canceled).
		if normalizedStatus != store.RunStatusSucceeded &&
			normalizedStatus != store.RunStatusFailed &&
			normalizedStatus != store.RunStatusCanceled {
			http.Error(w, fmt.Sprintf("status must be succeeded, failed, or canceled, got %s", req.Status), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to complete the run.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists.
		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Verify the node has a job assigned for this run.
		// Node assignment is tracked via jobs.node_id, not runs.node_id.
		jobs, err := st.ListJobsByRun(r.Context(), runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check jobs: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: jobs check failed", "run_id", req.RunID, "err", err)
			return
		}
		hasJobAssignment := false
		for _, job := range jobs {
			if job.NodeID.Valid && job.NodeID == nodeID {
				hasJobAssignment = true
				break
			}
		}
		if !hasJobAssignment {
			http.Error(w, "no job for this run assigned to this node", http.StatusForbidden)
			return
		}

		// Prepare stats field (default to empty JSON object if not provided).
		// Stats validation is shared between run-level and step-level completions.
		statsBytes := []byte("{}")
		if len(req.Stats) > 0 {
			// Validate that stats is valid JSON and a JSON object.
			if !json.Valid(req.Stats) {
				http.Error(w, "stats field must be valid JSON", http.StatusBadRequest)
				return
			}
			// Require JSON object for stats (not string/number/array/etc.).
			var tmp any
			if err := json.Unmarshal(req.Stats, &tmp); err != nil {
				http.Error(w, "invalid stats JSON", http.StatusBadRequest)
				return
			}
			if _, ok := tmp.(map[string]any); !ok {
				http.Error(w, "stats must be a JSON object", http.StatusBadRequest)
				return
			}
			statsBytes = req.Stats
		}

		// Step-level completion: retrieve the job and transition it to terminal state.
		job, err := st.GetJobByRunAndStepIndex(r.Context(), store.GetJobByRunAndStepIndexParams{
			RunID:     runID,
			StepIndex: float64(req.StepIndex),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: get job failed", "run_id", req.RunID, "step_index", req.StepIndex, "err", err)
			return
		}

		// Verify the job is assigned to the requesting node.
		if !job.NodeID.Valid || job.NodeID != nodeID {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the job is in 'running' status before transitioning to terminal state.
		if job.Status != store.JobStatusRunning {
			http.Error(w, fmt.Sprintf("job status is %s, expected running", job.Status), http.StatusConflict)
			return
		}

		// Map run terminal status (succeeded/failed/canceled) to JobStatus.
		var jobStatus store.JobStatus
		switch normalizedStatus {
		case store.RunStatusSucceeded:
			jobStatus = store.JobStatusSucceeded
		case store.RunStatusFailed:
			jobStatus = store.JobStatusFailed
		case store.RunStatusCanceled:
			jobStatus = store.JobStatusCanceled
		default:
			// Fallback for unexpected terminal states.
			jobStatus = store.JobStatusFailed
		}

		// Transition job status to terminal state (succeeded/failed/canceled).
		// Sets finished_at timestamp, duration_ms, and exit_code.
		err = st.UpdateJobCompletion(r.Context(), store.UpdateJobCompletionParams{
			ID:       job.ID,
			Status:   jobStatus,
			ExitCode: req.ExitCode,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: update failed", "run_id", req.RunID, "step_index", req.StepIndex, "node_id", nodeIDStr, "err", err)
			return
		}

		slog.Info("job completed",
			"run_id", req.RunID,
			"step_index", req.StepIndex,
			"node_id", nodeIDStr,
			"status", jobStatus,
			"exit_code", req.ExitCode,
			"stats_size", len(statsBytes),
		)

		// If gate job failed, check if healing jobs should be created.
		// This allows the server to dynamically insert healing jobs when gates fail.
		if jobStatus == store.JobStatusFailed {
			if err := maybeCreateHealingJobs(r.Context(), st, run, runID, req.StepIndex, jobs); err != nil {
				slog.Error("complete job: failed to create healing jobs",
					"run_id", req.RunID,
					"step_index", req.StepIndex,
					"err", err,
				)
			}
		}

		// Server-driven scheduling: after job succeeds or is skipped, schedule the next job.
		// This transitions the first 'created' job to 'scheduled' so it can be claimed.
		if jobStatus == store.JobStatusSucceeded || jobStatus == store.JobStatusSkipped {
			if _, err := st.ScheduleNextJob(r.Context(), runID); err != nil {
				// Log error but don't fail the job completion (job is already marked complete).
				// pgx.ErrNoRows means no more jobs to schedule, which is expected for the last job.
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to schedule next job",
						"run_id", req.RunID,
						"step_index", req.StepIndex,
						"err", err,
					)
				}
			}
		}

		// After completing a job, check if the run should transition to terminal state.
		// Derive the run's terminal status from the collective state of all jobs
		// instead of trusting the caller's status field.
		if err := maybeCompleteMultiStepRun(r.Context(), st, eventsService, run, runID); err != nil {
			// Log error but don't fail the job completion (job is already marked complete).
			slog.Error("complete job: failed to check run completion", "run_id", req.RunID, "step_index", req.StepIndex, "err", err)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// maybeCompleteMultiStepRun checks if all jobs of a multi-step run are complete
// and transitions the run to its terminal state (succeeded/failed/canceled).
// This function derives the run's terminal status from the collective state of
// all jobs instead of trusting the caller's status field.
//
// Status derivation rules:
// - If any job failed, the run is marked as failed.
// - If any job was canceled, the run is marked as canceled (unless a job failed).
// - If all jobs succeeded, the run is marked as succeeded.
// - If jobs are still created/scheduled/running, the run remains in running state.
func maybeCompleteMultiStepRun(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID pgtype.UUID) error {
	// Count the total number of jobs for this run.
	totalJobs, err := st.CountJobsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("count jobs: %w", err)
	}

	// Every run must have jobs. If there are no jobs, something is wrong.
	if totalJobs == 0 {
		return fmt.Errorf("run has no jobs")
	}

	// Count jobs by terminal status to determine the run's effective state.
	succeededCount, err := st.CountJobsByRunAndStatus(ctx, store.CountJobsByRunAndStatusParams{
		RunID:  runID,
		Status: store.JobStatusSucceeded,
	})
	if err != nil {
		return fmt.Errorf("count succeeded jobs: %w", err)
	}

	failedCount, err := st.CountJobsByRunAndStatus(ctx, store.CountJobsByRunAndStatusParams{
		RunID:  runID,
		Status: store.JobStatusFailed,
	})
	if err != nil {
		return fmt.Errorf("count failed jobs: %w", err)
	}

	canceledCount, err := st.CountJobsByRunAndStatus(ctx, store.CountJobsByRunAndStatusParams{
		RunID:  runID,
		Status: store.JobStatusCanceled,
	})
	if err != nil {
		return fmt.Errorf("count canceled jobs: %w", err)
	}

	// Calculate terminal jobs (succeeded + failed + canceled).
	terminalJobs := succeededCount + failedCount + canceledCount

	// If not all jobs are in terminal state, the run is still in progress.
	// Do not transition the run to a terminal state yet.
	if terminalJobs < totalJobs {
		slog.Debug("multi-step run still in progress",
			"run_id", runID,
			"total_jobs", totalJobs,
			"terminal_jobs", terminalJobs,
			"succeeded", succeededCount,
			"failed", failedCount,
			"canceled", canceledCount,
		)
		return nil
	}

	// All jobs are in terminal state. Derive the run's terminal status.
	// Priority: failed > canceled > succeeded.
	var runStatus store.RunStatus

	if failedCount > 0 {
		// At least one job failed: mark the run as failed.
		runStatus = store.RunStatusFailed
	} else if canceledCount > 0 {
		// At least one job was canceled (and no failures): mark the run as canceled.
		runStatus = store.RunStatusCanceled
	} else {
		// All jobs succeeded: mark the run as succeeded.
		runStatus = store.RunStatusSucceeded
	}

	slog.Info("multi-step run completing",
		"run_id", runID,
		"total_jobs", totalJobs,
		"succeeded", succeededCount,
		"failed", failedCount,
		"canceled", canceledCount,
		"derived_status", runStatus,
	)

	// Transition the run to its terminal status.
	// Use empty JSON object for stats (step-level stats are tracked per step).
	err = st.UpdateRunCompletion(ctx, store.UpdateRunCompletionParams{
		ID:     runID,
		Status: runStatus,
		Stats:  []byte("{}"),
	})
	if err != nil {
		return fmt.Errorf("update run completion: %w", err)
	}

	// Update job status to terminal and set finished_at/duration.
	if jobs, err := st.ListJobsByRun(ctx, runID); err == nil && len(jobs) > 0 {
		now := time.Now().UTC()
		var jobStatus store.JobStatus
		switch runStatus {
		case store.RunStatusSucceeded:
			jobStatus = store.JobStatusSucceeded
		case store.RunStatusFailed:
			jobStatus = store.JobStatusFailed
		case store.RunStatusCanceled:
			jobStatus = store.JobStatusCanceled
		default:
			jobStatus = store.JobStatusFailed
		}
		dur := int64(0)
		if jobs[0].StartedAt.Valid {
			d := now.Sub(jobs[0].StartedAt.Time).Milliseconds()
			if d > 0 {
				dur = d
			}
		}
		_ = st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         jobs[0].ID,
			Status:     jobStatus,
			StartedAt:  jobs[0].StartedAt,
			FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
			DurationMs: dur,
		})
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

// maybeCreateHealingJobs creates healing jobs and a re-gate job when a gate job fails.
// This is called when a gate job (pre_gate, post_gate, re_gate) completes with reason="build-gate".
//
// The function:
// 1. Finds the failed gate job by step_index
// 2. Verifies it's a gate job (pre_gate, post_gate, re_gate)
// 3. Checks if healing is configured in the run spec
// 4. Counts existing healing attempts to enforce retry limits
// 5. Creates healing jobs and a re-gate job at intermediate step_index values
//
// Float step_index enables dynamic job insertion:
//
//	pre-gate (1000) → FAIL → healing-0 (1100) → healing-1 (1200) → re-gate (1300) → mod-0 (2000)
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	run store.Run,
	runID pgtype.UUID,
	failedStepIndex domaintypes.StepIndex,
	jobs []store.Job,
) error {
	// Find the failed gate job by step_index.
	var failedJob *store.Job
	for i := range jobs {
		if jobs[i].StepIndex == float64(failedStepIndex) {
			failedJob = &jobs[i]
			break
		}
	}
	if failedJob == nil {
		slog.Debug("maybeCreateHealingJobs: no job found at step_index",
			"run_id", runID,
			"step_index", failedStepIndex,
		)
		return nil
	}

	// Parse job metadata to verify it's a gate job.
	var jobMeta modsapi.StageMetadata
	if len(failedJob.Meta) > 0 {
		if err := json.Unmarshal(failedJob.Meta, &jobMeta); err != nil {
			return fmt.Errorf("parse job metadata: %w", err)
		}
	}

	// Only create healing for gate jobs.
	if jobMeta.ModType != "pre_gate" && jobMeta.ModType != "post_gate" && jobMeta.ModType != "re_gate" {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", runID,
			"mod_type", jobMeta.ModType,
		)
		return nil
	}

	// Parse run spec to get healing configuration.
	var specMap map[string]any
	if len(run.Spec) > 0 && json.Valid(run.Spec) {
		if err := json.Unmarshal(run.Spec, &specMap); err != nil {
			return fmt.Errorf("parse run spec: %w", err)
		}
	}

	// Check if healing is configured.
	healingConfig, ok := specMap["build_gate_healing"].(map[string]any)
	if !ok {
		slog.Debug("maybeCreateHealingJobs: no healing config, skipping",
			"run_id", runID,
		)
		return nil
	}

	// Get healing mods list.
	healingMods, ok := healingConfig["mods"].([]any)
	if !ok || len(healingMods) == 0 {
		slog.Debug("maybeCreateHealingJobs: no healing mods configured",
			"run_id", runID,
		)
		return nil
	}

	// Get retry limit (default to 1 if not specified).
	retries := 1
	if r, ok := healingConfig["retries"].(float64); ok && r > 0 {
		retries = int(r)
	}

	// Count existing healing attempts by counting "heal" jobs for this run.
	healingAttempts := 0
	for _, job := range jobs {
		var meta modsapi.StageMetadata
		if len(job.Meta) > 0 {
			_ = json.Unmarshal(job.Meta, &meta)
		}
		if meta.ModType == "heal" {
			healingAttempts++
		}
	}

	// Check if retries exhausted.
	healingAttemptNumber := healingAttempts/len(healingMods) + 1 // 1-based attempt number
	if healingAttemptNumber > retries {
		slog.Info("maybeCreateHealingJobs: healing retries exhausted",
			"run_id", runID,
			"attempt", healingAttemptNumber,
			"max_retries", retries,
		)
		return nil
	}

	// Find the next job after the failed gate to calculate insertion range.
	nextStepIndex := float64(failedStepIndex) + 1000 // Default gap
	for _, job := range jobs {
		if job.StepIndex > float64(failedStepIndex) {
			if job.StepIndex < nextStepIndex {
				nextStepIndex = job.StepIndex
			}
		}
	}

	// Calculate step_index range for healing jobs.
	// Divide the gap between failed job and next job evenly.
	gapSize := nextStepIndex - float64(failedStepIndex)
	healingCount := len(healingMods)
	stepIncrement := gapSize / float64(healingCount+2) // +2 for re-gate and buffer

	slog.Info("maybeCreateHealingJobs: creating healing jobs",
		"run_id", runID,
		"failed_step_index", failedStepIndex,
		"next_step_index", nextStepIndex,
		"healing_count", healingCount,
		"attempt", healingAttemptNumber,
	)

	// Create healing jobs.
	// Server-driven scheduling: first healing job is 'scheduled' (runs immediately),
	// subsequent jobs are 'created' (wait for server to schedule after prior completes).
	for i, modInterface := range healingMods {
		modMap, ok := modInterface.(map[string]any)
		if !ok {
			continue
		}

		// Extract healing mod image.
		modImage := ""
		if img, ok := modMap["image"].(string); ok {
			modImage = strings.TrimSpace(img)
		}

		// Calculate step_index for this healing job.
		healStepIndex := float64(failedStepIndex) + stepIncrement*float64(i+1)

		// Build job metadata.
		jobMeta := modsapi.StageMetadata{
			ModType:  "heal",
			ModImage: modImage,
		}
		metaBytes, err := json.Marshal(jobMeta)
		if err != nil {
			return fmt.Errorf("marshal healing job metadata: %w", err)
		}

		// First healing job is scheduled (ready to claim), others are created.
		jobStatus := store.JobStatusCreated
		if i == 0 {
			jobStatus = store.JobStatusScheduled
		}

		// Create the healing job.
		jobName := fmt.Sprintf("heal-%d-%d", healingAttemptNumber, i)
		_, err = st.CreateJob(ctx, store.CreateJobParams{
			RunID:     runID,
			Name:      jobName,
			Status:    jobStatus,
			StepIndex: healStepIndex,
			Meta:      metaBytes,
		})
		if err != nil {
			return fmt.Errorf("create healing job %s: %w", jobName, err)
		}

		slog.Info("created healing job",
			"run_id", runID,
			"job_name", jobName,
			"step_index", healStepIndex,
			"status", jobStatus,
			"image", modImage,
		)
	}

	// Create re-gate job after healing jobs - starts as 'created'.
	reGateStepIndex := float64(failedStepIndex) + stepIncrement*float64(healingCount+1)
	reGateMeta := modsapi.StageMetadata{
		ModType: "re_gate",
	}
	reGateMetaBytes, err := json.Marshal(reGateMeta)
	if err != nil {
		return fmt.Errorf("marshal re-gate metadata: %w", err)
	}

	reGateName := fmt.Sprintf("re-gate-%d", healingAttemptNumber)
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		RunID:     runID,
		Name:      reGateName,
		Status:    store.JobStatusCreated,
		StepIndex: reGateStepIndex,
		Meta:      reGateMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

	slog.Info("created re-gate job",
		"run_id", runID,
		"job_name", reGateName,
		"step_index", reGateStepIndex,
	)

	return nil
}
