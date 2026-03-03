package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// completeJobRequest represents the request body for job completion.
// This is a simpler contract than the node-based endpoint since job_id
// is in the URL path and node identity comes from mTLS.
type completeJobRequest struct {
	Status     string          `json:"status"`                 // Terminal status: Success, Fail, or Cancelled
	ExitCode   *int32          `json:"exit_code,omitempty"`    // Exit code from job execution
	Stats      json.RawMessage `json:"stats,omitempty"`        // Optional job statistics (must be JSON object)
	RepoSHAOut string          `json:"repo_sha_out,omitempty"` // Optional lowercase 40-hex output SHA reported by node.
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
func completeJobHandler(st store.Store, eventsService *server.EventsService, bp *blobpersist.Service) http.HandlerFunc {
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

		repoSHAOut := ""
		if candidate := strings.TrimSpace(req.RepoSHAOut); candidate != "" {
			if !sha40Pattern.MatchString(candidate) {
				httpErr(w, http.StatusBadRequest, "repo_sha_out must match ^[0-9a-f]{40}$")
				return
			}
			repoSHAOut = candidate
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

		if normalizedStatus == store.JobStatusSuccess && job.NextID != nil {
			if !sha40Pattern.MatchString(job.RepoShaIn) {
				httpErr(w, http.StatusConflict, "job repo_sha_in must match ^[0-9a-f]{40}$ for chain progression")
				return
			}
			if repoSHAOut == "" {
				httpErr(w, http.StatusBadRequest, "repo_sha_out is required for successful jobs with next_id")
				return
			}
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
		persistedJobMeta := append([]byte(nil), job.Meta...)

		// Transition job status to terminal state.
		// Sets finished_at timestamp, duration_ms, and exit_code.
		// When job_meta is present in stats, persist it into jobs.meta JSONB.
		// The job_meta has already been validated via ValidateJobMeta() above.
		if statsPayload.HasJobMeta() {
			mergedMeta, mergeErr := mergeCompletionJobMeta(job.Meta, statsPayload.JobMeta)
			if mergeErr != nil {
				httpErr(w, http.StatusInternalServerError, "failed to merge job metadata: %v", mergeErr)
				slog.Error("complete job: merge metadata failed",
					"job_id", jobID,
					"err", mergeErr,
				)
				return
			}
			err = st.UpdateJobCompletionWithMeta(ctx, store.UpdateJobCompletionWithMetaParams{
				ID:         job.ID,
				Status:     jobStatus,
				ExitCode:   req.ExitCode,
				Meta:       mergedMeta,
				RepoShaOut: repoSHAOut,
			})
			persistedJobMeta = mergedMeta
		} else {
			err = st.UpdateJobCompletion(ctx, store.UpdateJobCompletionParams{
				ID:         job.ID,
				Status:     jobStatus,
				ExitCode:   req.ExitCode,
				RepoShaOut: repoSHAOut,
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

		// Load run details only for run-dependent follow-up operations (healing insertion
		// and run-level terminal reconciliation). Other side effects must not be gated
		// by run lookup failures.
		var cachedRun store.Run
		hasCachedRun := false
		loadRunForPostCompletion := func(purpose string) (store.Run, bool) {
			if hasCachedRun {
				return cachedRun, true
			}
			run, runErr := st.GetRun(ctx, runID)
			if runErr == nil {
				cachedRun = run
				hasCachedRun = true
				return run, true
			}

			// Retry once for transient read failures before giving up.
			slog.Warn("complete job: get run failed, retrying",
				"job_id", jobID,
				"run_id", runID,
				"purpose", purpose,
				"err", runErr,
			)
			run, retryErr := st.GetRun(ctx, runID)
			if retryErr != nil {
				slog.Error("complete job: get run failed",
					"job_id", jobID,
					"run_id", runID,
					"purpose", purpose,
					"err", retryErr,
				)
				return store.Run{}, false
			}
			cachedRun = run
			hasCachedRun = true
			return run, true
		}

		// When a job fails, either:
		// - If it is a gate job, invoke maybeCreateHealingJobs (which may create healing/re-gate
		//   jobs or cancel remaining jobs when healing is not configured or exhausted).
		// - If it is a non-gate job (mig/heal), cancel remaining non-terminal jobs so the run
		//   can reach a terminal state instead of leaving jobs stranded.
		// v1 uses Fail instead of failed.
		if jobStatus == store.JobStatusFail {
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
				if errMsg := formatStackGateError(jobType, persistedJobMeta); errMsg != nil {
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
				run, ok := loadRunForPostCompletion("healing insertion")
				if ok {
					if healErr := maybeCreateHealingJobs(ctx, st, bp, run, job); healErr != nil {
						slog.Error("complete job: failed to create healing jobs",
							"job_id", jobID,
							"next_id", job.NextID,
							"err", healErr,
						)
					}
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
		if jobStatus == store.JobStatusCancelled {
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
			if promoteErr := maybePromoteReGateRecoveryCandidate(ctx, st, job, persistedJobMeta); promoteErr != nil {
				slog.Error("complete job: failed to promote validated re-gate candidate",
					"job_id", jobID,
					"repo_id", job.RepoID,
					"err", promoteErr,
				)
			}
			if refreshErr := maybeRefreshNextReGateRecoveryCandidate(ctx, st, bp, job); refreshErr != nil {
				slog.Error("complete job: failed to refresh next re-gate recovery candidate",
					"job_id", jobID,
					"repo_id", job.RepoID,
					"err", refreshErr,
				)
			}
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
		if !isMRJob {
			// Update run_repos.status if all jobs for this repo attempt are terminal.
			repoUpdated, repoErr := recovery.MaybeUpdateRunRepoStatus(ctx, st, job.RunID, job.RepoID, job.Attempt)
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
				run, ok := loadRunForPostCompletion("run completion reconciliation")
				if ok {
					if _, completeErr := recovery.MaybeCompleteRunIfAllReposTerminal(ctx, st, eventsService, run); completeErr != nil {
						slog.Error("complete job: failed to check run completion",
							"job_id", jobID,
							"next_id", job.NextID,
							"err", completeErr,
						)
					}
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
		if mrURL != "" && isMRJob {
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

func mergeCompletionJobMeta(existingRaw, incomingRaw []byte) ([]byte, error) {
	incoming, err := contracts.UnmarshalJobMeta(incomingRaw)
	if err != nil {
		return nil, err
	}

	existing, err := contracts.UnmarshalJobMeta(existingRaw)
	if err != nil {
		return incomingRaw, nil
	}

	merged := false
	if incoming.Recovery == nil && existing.Recovery != nil {
		incoming.Recovery = cloneRecoveryMetadataForCompletion(existing.Recovery)
		merged = true
	}
	if incoming.Gate != nil && incoming.Gate.Recovery == nil && existing.Gate != nil && existing.Gate.Recovery != nil {
		incoming.Gate.Recovery = cloneRecoveryMetadataForCompletion(existing.Gate.Recovery)
		merged = true
	}
	if !merged {
		return incomingRaw, nil
	}
	return contracts.MarshalJobMeta(incoming)
}

func cloneRecoveryMetadataForCompletion(src *contracts.BuildGateRecoveryMetadata) *contracts.BuildGateRecoveryMetadata {
	if src == nil {
		return nil
	}
	out := *src
	if src.Confidence != nil {
		v := *src.Confidence
		out.Confidence = &v
	}
	if src.CandidatePromoted != nil {
		v := *src.CandidatePromoted
		out.CandidatePromoted = &v
	}
	if len(src.Expectations) > 0 {
		out.Expectations = append([]byte(nil), src.Expectations...)
	}
	if len(src.CandidateGateProfile) > 0 {
		out.CandidateGateProfile = append([]byte(nil), src.CandidateGateProfile...)
	}
	return &out
}

func maybePromoteReGateRecoveryCandidate(
	ctx context.Context,
	st store.Store,
	job store.Job,
	rawMeta []byte,
) error {
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeReGate {
		return nil
	}
	meta, err := contracts.UnmarshalJobMeta(rawMeta)
	if err != nil {
		return nil
	}
	recovery := meta.Recovery
	if recovery == nil && meta.Gate != nil {
		recovery = meta.Gate.Recovery
	}
	if recovery == nil {
		return nil
	}
	if recovery.CandidateValidationStatus != contracts.RecoveryCandidateStatusValid {
		return nil
	}
	if len(bytes.TrimSpace(recovery.CandidateGateProfile)) == 0 {
		return nil
	}
	if recovery.CandidatePromoted != nil && *recovery.CandidatePromoted {
		return nil
	}

	candidatePromoted := true
	recovery.CandidatePromoted = &candidatePromoted
	promotedMeta, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return err
	}

	artifactPath := strings.TrimSpace(recovery.CandidateArtifactPath)
	if artifactPath == "" {
		artifactPath = contracts.GateProfileCandidateArtifactPath
	}
	schemaID := strings.TrimSpace(recovery.CandidateSchemaID)
	if schemaID == "" {
		schemaID = contracts.GateProfileCandidateSchemaID
	}
	prepArtifacts, err := json.Marshal(map[string]any{
		"source":        "recovery_candidate",
		"schema_id":     schemaID,
		"artifact_path": artifactPath,
		"job_id":        job.ID.String(),
	})
	if err != nil {
		return err
	}

	_, err = st.PromoteReGateRecoveryCandidateGateProfile(ctx, store.PromoteReGateRecoveryCandidateGateProfileParams{
		ID:                   job.ID,
		JobMeta:              promotedMeta,
		GateProfile:          recovery.CandidateGateProfile,
		GateProfileArtifacts: prepArtifacts,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func maybeRefreshNextReGateRecoveryCandidate(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	healJob store.Job,
) error {
	if domaintypes.JobType(healJob.JobType) != domaintypes.JobTypeHeal {
		return nil
	}
	if bp == nil || healJob.NextID == nil {
		return nil
	}

	reGateJob, err := st.GetJob(ctx, *healJob.NextID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if domaintypes.JobType(reGateJob.JobType) != domaintypes.JobTypeReGate {
		return nil
	}

	meta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		return nil
	}
	recovery := meta.Recovery
	if recovery == nil && meta.Gate != nil {
		recovery = meta.Gate.Recovery
	}
	if recovery == nil {
		return nil
	}
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return nil
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   reGateJob.RunID,
		RepoID:  reGateJob.RepoID,
		Attempt: reGateJob.Attempt,
	})
	if err != nil {
		return err
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, item := range jobs {
		jobsByID[item.ID] = item
	}
	if refreshed, ok := jobsByID[reGateJob.ID]; ok {
		reGateJob = refreshed
	}

	var detectedExpectation *contracts.StackExpectation
	if prevHeal := predecessorOf(reGateJob.ID, jobsByID); prevHeal != nil {
		if failedGate := predecessorOf(prevHeal.ID, jobsByID); failedGate != nil {
			_, _, detectedExpectation = resolveFailedGateRecoveryContext(*failedGate)
		}
	}

	updatedRecovery := cloneRecoveryMetadata(recovery)
	evaluateAndAttachInfraCandidate(ctx, bp, reGateJob.RunID, reGateJob, jobsByID, detectedExpectation, updatedRecovery)
	if meta.Recovery != nil {
		meta.Recovery = updatedRecovery
	}
	if meta.Gate != nil && meta.Gate.Recovery != nil {
		meta.Gate.Recovery = updatedRecovery
	}
	if meta.Recovery == nil && (meta.Gate == nil || meta.Gate.Recovery == nil) {
		meta.Recovery = updatedRecovery
	}

	updatedMeta, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return err
	}
	return st.UpdateJobMeta(ctx, store.UpdateJobMetaParams{
		ID:   reGateJob.ID,
		Meta: updatedMeta,
	})
}
