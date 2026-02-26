package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

// completeJobRequest represents the request body for job completion.
// This is a simpler contract than the node-based endpoint since job_id
// is in the URL path and node identity comes from mTLS.
type completeJobRequest struct {
	Status   string          `json:"status"`              // Terminal status: Success, Fail, or Cancelled
	ExitCode *int32          `json:"exit_code,omitempty"` // Exit code from job execution
	Stats    json.RawMessage `json:"stats,omitempty"`     // Optional job statistics (must be JSON object)
}

// completeJobHandler marks a job as completed with terminal status and stats.
// This endpoint simplifies the node → server contract by addressing jobs directly
// via the URL path (/v1/jobs/{job_id}/complete) rather than requiring run_id and
// next_id in the request body.
//
// Authentication: Node identity is derived from the mTLS client certificate.
// The handler verifies that the job is assigned to the calling node.
//
// Request body:
//
//	{
//	  "status": "Success" | "Fail" | "Cancelled",
//	  "exit_code": 0,
//	  "stats": { ... }
//	}
//
// Response: 204 No Content on success.
func completeJobHandler(st store.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract job_id from URL path parameter using domain type helper.
		jobID, err := parseParam[domaintypes.JobID](r, "job_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Extract caller identity from context (set by auth middleware).
		_, ok := auth.IdentityFromContext(ctx)
		if !ok {
			httpErr(w, http.StatusUnauthorized, "unauthorized: no identity in context")
			return
		}

		// Decode request body for status, exit_code, and stats with strict validation.
		var req completeJobRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate and convert status to canonical JobStatus type.
		// v1 uses capitalized job status values: Success, Fail, Cancelled.
		if strings.TrimSpace(req.Status) == "" {
			httpErr(w, http.StatusBadRequest, "status is required")
			return
		}
		normalizedStatus, err := store.ConvertToJobStatus(strings.TrimSpace(req.Status))
		if err != nil {
			httpErr(w, http.StatusBadRequest, "invalid status: %v", err)
			return
		}

		// Validate that status is a terminal job state (Success, Fail, or Cancelled).
		if normalizedStatus != store.JobStatusSuccess &&
			normalizedStatus != store.JobStatusFail &&
			normalizedStatus != store.JobStatusCancelled {
			httpErr(w, http.StatusBadRequest, "status must be Success, Fail, or Cancelled, got %s", req.Status)
			return
		}

		// Validate and parse stats field into typed JobStatsPayload.
		// This replaces untyped map[string]any decoding with a structured approach
		// that provides compile-time type safety and schema validation.
		statsBytes := []byte("{}")
		var statsPayload JobStatsPayload
		if len(req.Stats) > 0 {
			// First, validate that stats is valid JSON.
			if !json.Valid(req.Stats) {
				httpErr(w, http.StatusBadRequest, "stats field must be valid JSON")
				return
			}

			// Verify stats is a JSON object (not array, string, number, etc.).
			// We do a quick type check before unmarshaling into the typed struct.
			var rawCheck json.RawMessage
			if err := json.Unmarshal(req.Stats, &rawCheck); err != nil {
				httpErr(w, http.StatusBadRequest, "invalid stats JSON")
				return
			}
			// Trim whitespace and check first character for object delimiter.
			trimmed := strings.TrimSpace(string(rawCheck))
			if len(trimmed) == 0 || trimmed[0] != '{' {
				httpErr(w, http.StatusBadRequest, "stats must be a JSON object")
				return
			}

			// Unmarshal into typed JobStatsPayload struct.
			// Unknown fields are silently ignored (forward compatibility).
			if err := json.Unmarshal(req.Stats, &statsPayload); err != nil {
				httpErr(w, http.StatusBadRequest, "invalid stats payload: %v", err)
				return
			}
			statsBytes = req.Stats

			// Validate job_meta via contracts.UnmarshalJobMeta when present.
			// This ensures structured metadata conforms to the JobMeta schema
			// before persisting to jobs.meta JSONB.
			if err := statsPayload.ValidateJobMeta(); err != nil {
				httpErr(w, http.StatusBadRequest, "%s", err)
				return
			}
			if err := statsPayload.ValidateJobResources(); err != nil {
				httpErr(w, http.StatusBadRequest, "%s", err)
				return
			}
		}

		// Look up the job by job_id (KSUID-backed).
		job, err := st.GetJob(ctx, jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "job not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get job: %v", err)
			slog.Error("complete job: get job failed", "job_id", jobID, "err", err)
			return
		}

		// Derive run_id from the job for run completion checks.
		runID := job.RunID

		// Derive node ID from required header. auth middleware already enforces
		// presence for worker-role callers; this handler performs an additional
		// check and uses the value for job ownership validation.
		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			httpErr(w, http.StatusBadRequest, "PLOY_NODE_UUID header is required")
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			httpErr(w, http.StatusBadRequest, "invalid PLOY_NODE_UUID header")
			return
		}

		// Verify the job is assigned to the calling node.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			httpErr(w, http.StatusForbidden, "job not assigned to this node")
			return
		}

		// Verify the job is in 'Running' status before transitioning to terminal state.
		// v1 uses capitalized status values.
		if job.Status != store.JobStatusRunning {
			httpErr(w, http.StatusConflict, "job status is %s, expected Running", job.Status)
			return
		}

		// Persist per-job resource consumption metrics when provided.
		if statsPayload.HasJobResources() {
			res := statsPayload.JobResources
			if err := st.UpsertJobMetric(ctx, store.UpsertJobMetricParams{
				NodeID:            nodeIDHeader,
				JobID:             job.ID,
				CpuConsumedNs:     res.CPUConsumedNs,
				DiskConsumedBytes: res.DiskConsumedBytes,
				MemConsumedBytes:  res.MemConsumedBytes,
			}); err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to persist job metrics: %v", err)
				slog.Error("complete job: persist job metrics failed",
					"job_id", jobID,
					"node_id", nodeIDHeader,
					"err", err,
				)
				return
			}
		}

		// Use the validated job status directly (already a JobStatus type).
		jobStatus := normalizedStatus

		// Transition job status to terminal state.
		// Sets finished_at timestamp, duration_ms, and exit_code.
		// When job_meta is present in stats, persist it into jobs.meta JSONB.
		// The job_meta has already been validated via ValidateJobMeta() above.
		if statsPayload.HasJobMeta() {
			err = st.UpdateJobCompletionWithMeta(ctx, store.UpdateJobCompletionWithMetaParams{
				ID:       job.ID,
				Status:   jobStatus,
				ExitCode: req.ExitCode,
				Meta:     statsPayload.JobMeta,
			})
		} else {
			err = st.UpdateJobCompletion(ctx, store.UpdateJobCompletionParams{
				ID:       job.ID,
				Status:   jobStatus,
				ExitCode: req.ExitCode,
			})
		}
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to complete job: %v", err)
			slog.Error("complete job: update failed",
				"job_id", jobID,
				"next_id", job.NextID,
				"node_id", nodeIDHeader,
				"err", err,
			)
			return
		}

		slog.Info("job completed",
			"job_id", jobID,
			"next_id", job.NextID,
			"node_id", nodeIDHeader,
			"status", jobStatus,
			"exit_code", req.ExitCode,
			"stats_size", len(statsBytes),
		)

		// Fetch the run for post-completion processing.
		run, err := st.GetRun(ctx, runID)
		if err != nil {
			// Log error but don't fail the job completion (job is already marked complete).
			slog.Error("complete job: get run failed", "job_id", jobID, "run_id", runID, "err", err)
		}

		// When a job fails, either:
		// - If it is a gate job, invoke maybeCreateHealingJobs (which may create healing/re-gate
		//   jobs or cancel remaining jobs when healing is not configured or exhausted).
		// - If it is a non-gate job (mig/heal), cancel remaining non-terminal jobs so the run
		//   can reach a terminal state instead of leaving jobs stranded.
		// v1 uses Fail instead of failed.
		if jobStatus == store.JobStatusFail && err == nil {
			if errMsg := formatExit137Error(job.Name, req.ExitCode); errMsg != nil {
				if updateErr := st.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
					RunID:     job.RunID,
					RepoID:    job.RepoID,
					LastError: errMsg,
				}); updateErr != nil {
					slog.Error("complete job: failed to set repo last_error for exit code 137",
						"job_id", jobID,
						"repo_id", job.RepoID,
						"err", updateErr,
					)
				}
			}

			jobType := domaintypes.JobType(job.JobType)
			if err := jobType.Validate(); err != nil {
				slog.Error("complete job: invalid job_type in job record; treating as non-gate for failure handling",
					"job_id", jobID,
					"job_type", job.JobType,
					"err", err,
				)
				jobType = ""
			}
			switch jobType {
			case domaintypes.JobTypeMR:
				// MR jobs are best-effort and must not trigger healing or
				// cancellation of other jobs when they fail.
				slog.Warn("complete job: MR job failed; ignoring for run-level failure handling",
					"job_id", jobID,
					"next_id", job.NextID,
				)
			case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
				// Set last_error for Stack Gate failures
				if statsPayload.HasJobMeta() {
					if errMsg := formatStackGateError(jobType, statsPayload.JobMeta); errMsg != nil {
						if updateErr := st.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{
							RunID:     job.RunID,
							RepoID:    job.RepoID,
							LastError: errMsg,
						}); updateErr != nil {
							slog.Error("complete job: failed to set repo last_error",
								"job_id", jobID,
								"repo_id", job.RepoID,
								"err", updateErr,
							)
						}
					}
				}
				if healErr := maybeCreateHealingJobs(ctx, st, run, job); healErr != nil {
					slog.Error("complete job: failed to create healing jobs",
						"job_id", jobID,
						"next_id", job.NextID,
						"err", healErr,
					)
				}
			default:
				if err := cancelRemainingJobsAfterFailure(ctx, st, job); err != nil {
					slog.Error("complete job: failed to cancel remaining jobs after non-gate failure",
						"job_id", jobID,
						"next_id", job.NextID,
						"err", err,
					)
				}
			}
		}

		// When a job is cancelled (not by explicit run cancel), ensure the repo attempt
		// reaches a terminal state by cancelling remaining jobs after this step.
		// This is critical for policy-driven cancellations (e.g., stack detection required).
		if jobStatus == store.JobStatusCancelled && err == nil {
			jobType := domaintypes.JobType(job.JobType)
			if jobType.Validate() == nil && jobType == domaintypes.JobTypeMR {
				// MR jobs are best-effort and must not affect run-level progression.
			} else {
				if cerr := cancelRemainingJobsAfterFailure(ctx, st, job); cerr != nil {
					slog.Error("complete job: failed to cancel remaining jobs after cancellation",
						"job_id", jobID,
						"next_id", job.NextID,
						"err", cerr,
					)
				}
			}
		}

		// Server-driven scheduling: after job succeeds, promote the linked successor.
		if jobStatus == store.JobStatusSuccess {
			if job.NextID != nil {
				if _, err := st.PromoteJobByIDIfUnblocked(ctx, *job.NextID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
					slog.Error("complete job: failed to promote next linked job",
						"job_id", jobID,
						"next_id", job.NextID,
						"err", err,
					)
				}
			}
		}

		// v1 repo-scoped progression:
		// After completing a job, check if the repo attempt has reached a terminal state.
		// MR jobs (job_type='mr') are auxiliary and must not affect run_repos.status derivation.
		jobJobType := domaintypes.JobType(job.JobType)
		isMRJob := jobJobType.Validate() == nil && jobJobType == domaintypes.JobTypeMR
		if err == nil && !isMRJob {
			// Update run_repos.status if all jobs for this repo attempt are terminal.
			repoUpdated, repoErr := maybeUpdateRunRepoStatus(ctx, st, job.RunID, job.RepoID, job.Attempt)
			if repoErr != nil {
				slog.Error("complete job: failed to check repo completion",
					"job_id", jobID,
					"repo_id", job.RepoID,
					"attempt", job.Attempt,
					"err", repoErr,
				)
			}

			// If the repo reached terminal state, check if the run should transition to Finished.
			// runs.status becomes Finished when all repos are terminal.
			if repoUpdated {
				if completeErr := maybeCompleteRunIfAllReposTerminal(ctx, st, eventsService, run, runID); completeErr != nil {
					slog.Error("complete job: failed to check run completion",
						"job_id", jobID,
						"next_id", job.NextID,
						"err", completeErr,
					)
				}
			}
		}

		// For MR jobs that reported an MR URL in stats.metadata.mr_url, merge the
		// URL into runs.stats.metadata.mr_url so that GET /v1/runs/{id}/status can
		// expose it via RunStats.MRURL() and CLI commands can display it. This is a
		// best-effort update and does not affect run status.
		// We use the typed statsPayload.MRURL() accessor instead of map[string]any casting.
		// Note: jobJobType and isMRJob were already computed above for repo-scoped progression.
		mrURL := statsPayload.MRURL()
		if err == nil && mrURL != "" && isMRJob {
			if updateErr := st.UpdateRunStatsMRURL(ctx, store.UpdateRunStatsMRURLParams{
				ID:    runID,
				MrUrl: mrURL,
			}); updateErr != nil {
				slog.Error("complete job: failed to merge MR URL into run stats",
					"job_id", jobID,
					"run_id", runID,
					"err", updateErr,
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
