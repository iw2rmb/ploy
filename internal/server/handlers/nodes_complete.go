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

		// Decode request body to get run_id, job_id, status, exit_code, stats, and step_index.
		// Nodeagent includes job_id to identify which job is being completed (avoids float equality issues).
		// step_index is retained for logging/diagnostics but job_id is the authoritative lookup key.
		var req struct {
			RunID     domaintypes.RunID     `json:"run_id"`
			JobID     domaintypes.JobID     `json:"job_id"` // Job ID for completion (authoritative lookup key)
			Status    string                `json:"status"`
			ExitCode  *int32                `json:"exit_code,omitempty"` // Exit code from job execution
			Stats     json.RawMessage       `json:"stats,omitempty"`
			StepIndex domaintypes.StepIndex `json:"step_index"` // Retained for logging/compat
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

		// Validate job_id is present (required for job lookup).
		if req.JobID.IsZero() {
			http.Error(w, "job_id is required", http.StatusBadRequest)
			return
		}

		// Parse and validate job_id as UUID.
		jobID := domaintypes.ToPGUUID(req.JobID.String())
		if !jobID.Valid {
			http.Error(w, "invalid job_id: invalid uuid", http.StatusBadRequest)
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

		// Job-level completion: retrieve the job by job_id and transition it to terminal state.
		// Using job_id avoids float equality issues with step_index.
		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("complete job: get job failed", "run_id", req.RunID, "job_id", req.JobID, "err", err)
			return
		}

		// Verify the job belongs to the specified run.
		if job.RunID != runID {
			http.Error(w, "job does not belong to this run", http.StatusBadRequest)
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
			slog.Error("complete job: update failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", job.StepIndex, "node_id", nodeIDStr, "err", err)
			return
		}

		slog.Info("job completed",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"step_index", job.StepIndex,
			"node_id", nodeIDStr,
			"status", jobStatus,
			"exit_code", req.ExitCode,
			"stats_size", len(statsBytes),
		)

		// If gate job failed, check if healing jobs should be created.
		// This allows the server to dynamically insert healing jobs when gates fail.
		// Fetch jobs for healing job creation since we no longer have the pre-check.
		if jobStatus == store.JobStatusFailed {
			jobs, jobsErr := st.ListJobsByRun(r.Context(), runID)
			if jobsErr != nil {
				slog.Error("complete job: failed to list jobs for healing",
					"run_id", req.RunID,
					"job_id", req.JobID,
					"err", jobsErr,
				)
			} else if err := maybeCreateHealingJobs(r.Context(), st, run, runID, domaintypes.StepIndex(job.StepIndex), jobs); err != nil {
				slog.Error("complete job: failed to create healing jobs",
					"run_id", req.RunID,
					"job_id", req.JobID,
					"step_index", job.StepIndex,
					"err", err,
				)
			}
		}

		// If a healing job itself fails, cancel remaining non-terminal jobs so the
		// run can transition to a terminal state instead of leaving jobs stranded
		// in created/pending status.
		if jobStatus == store.JobStatusFailed {
			var meta modsapi.StageMetadata
			if len(job.Meta) > 0 {
				_ = json.Unmarshal(job.Meta, &meta)
			}
			if meta.ModType == "heal" {
				jobs, jobsErr := st.ListJobsByRun(r.Context(), runID)
				if jobsErr != nil {
					slog.Error("complete job: failed to list jobs for healing cancellation",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"err", jobsErr,
					)
				} else if err := cancelRemainingJobsAfterExhaustedHealing(r.Context(), st, runID, domaintypes.StepIndex(job.StepIndex), jobs); err != nil {
					slog.Error("complete job: failed to cancel remaining jobs after healing failure",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"step_index", job.StepIndex,
						"err", err,
					)
				}
			}
		}

		// Server-driven scheduling: after job succeeds or is skipped, schedule the next job.
		// This transitions the first 'created' job to 'pending' so it can be claimed.
		if jobStatus == store.JobStatusSucceeded || jobStatus == store.JobStatusSkipped {
			if _, err := st.ScheduleNextJob(r.Context(), runID); err != nil {
				// Log error but don't fail the job completion (job is already marked complete).
				// pgx.ErrNoRows means no more jobs to schedule, which is expected for the last job.
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to schedule next job",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"step_index", job.StepIndex,
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
			slog.Error("complete job: failed to check run completion", "run_id", req.RunID, "job_id", req.JobID, "step_index", job.StepIndex, "err", err)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

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
			job.Status == store.JobStatusCanceled
		if isTerminal {
			terminalJobs++
		}

		// Track canceled jobs for fallback precedence.
		if job.Status == store.JobStatusCanceled {
			hasCanceled = true
		}

		// Parse job metadata to determine if this is a gate job.
		isGate := modsapi.IsGateJob(job.Meta)

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

		// When healing retries are exhausted and the gate still fails, cancel
		// all remaining non-terminal jobs for the run so the control plane
		// can derive a terminal run state and avoid leaving mods/post-gate
		// jobs stranded in created/pending state.
		if err := cancelRemainingJobsAfterExhaustedHealing(ctx, st, runID, failedStepIndex, jobs); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs after exhausted healing",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
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
	// Server-driven scheduling: first healing job is 'pending' (runs immediately),
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

		// First healing job is pending (ready to claim), others are created.
		jobStatus := store.JobStatusCreated
		if i == 0 {
			jobStatus = store.JobStatusPending
		}

		// Create the healing job.
		jobName := fmt.Sprintf("heal-%d-%d", healingAttemptNumber, i)
		_, err = st.CreateJob(ctx, store.CreateJobParams{
			RunID:     runID,
			Name:      jobName,
			ModType:   "heal",
			ModImage:  modImage,
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
		ModType:   "re_gate",
		ModImage:  "",
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

// cancelRemainingJobsAfterExhaustedHealing cancels all non-terminal jobs with
// step_index greater than the failed gate's step_index. This ensures that once
// healing retries are exhausted and the gate still fails, remaining mods and
// post-gate jobs are not left stranded in created/pending state.
func cancelRemainingJobsAfterExhaustedHealing(
	ctx context.Context,
	st store.Store,
	runID pgtype.UUID,
	failedStepIndex domaintypes.StepIndex,
	jobs []store.Job,
) error {
	now := time.Now().UTC()

	for _, job := range jobs {
		if job.StepIndex <= float64(failedStepIndex) {
			continue
		}

		switch job.Status {
		case store.JobStatusSucceeded, store.JobStatusFailed, store.JobStatusCanceled, store.JobStatusSkipped:
			continue
		}

		startedAt := job.StartedAt
		var durationMs int64
		if job.StartedAt.Valid {
			durationMs = now.Sub(job.StartedAt.Time).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}

		finishedAt := pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		}

		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     store.JobStatusCanceled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", uuid.UUID(job.ID.Bytes).String(), err)
		}

		slog.Info("canceled job after exhausted healing",
			"run_id", runID,
			"job_id", uuid.UUID(job.ID.Bytes).String(),
			"step_index", job.StepIndex,
		)
	}

	return nil
}
