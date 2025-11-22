package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	// syncWaitTimeout is how long the validate endpoint waits for job completion
	// before returning a job ID for async polling.
	syncWaitTimeout = 30 * time.Second
	// pollInterval is how often we check job status during sync wait.
	pollInterval = 500 * time.Millisecond
)

// validateBuildGateHandler returns an HTTP handler for POST /v1/buildgate/validate.
// Accepts BuildGateValidateRequest, creates a job, and either:
// - Returns result immediately if completed within syncWaitTimeout
// - Returns job ID for async polling via GET /v1/buildgate/jobs/{id}
func validateBuildGateHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req contracts.BuildGateValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate request.
		if err := req.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("validation error: %v", err), http.StatusBadRequest)
			return
		}

		// Serialize request to JSONB.
		requestPayload, err := json.Marshal(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to serialize request: %v", err), http.StatusInternalServerError)
			slog.Error("validate buildgate: serialize request failed", "err", err)
			return
		}

		// Create job in database.
		job, err := st.CreateBuildGateJob(r.Context(), requestPayload)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create job: %v", err), http.StatusInternalServerError)
			slog.Error("validate buildgate: create job failed", "err", err)
			return
		}

		jobID := uuid.UUID(job.ID.Bytes).String()
		slog.Info("buildgate job created", "job_id", jobID)

		// Wait for job completion (with timeout).
		ctx, cancel := context.WithTimeout(r.Context(), syncWaitTimeout)
		defer cancel()

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Timeout - return job ID for async polling.
				resp := contracts.BuildGateValidateResponse{
					JobID:  jobID,
					Status: job.Status, // No cast needed: contracts.BuildGateJobStatus is now an alias for store.BuildgateJobStatus
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					slog.Error("validate buildgate: encode async response failed", "err", err)
				}
				slog.Info("buildgate job accepted (async)", "job_id", jobID)
				return

			case <-ticker.C:
				// Poll for job completion.
				job, err = st.GetBuildGateJob(r.Context(), job.ID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						http.Error(w, "job not found", http.StatusNotFound)
						return
					}
					http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
					slog.Error("validate buildgate: poll job failed", "job_id", jobID, "err", err)
					return
				}

				// Check if job is terminal.
				if job.Status == store.BuildgateJobStatusCompleted || job.Status == store.BuildgateJobStatusFailed {
					// Job completed - return result synchronously.
					var result *contracts.BuildGateStageMetadata
					if len(job.Result) > 0 {
						result = &contracts.BuildGateStageMetadata{}
						if err := json.Unmarshal(job.Result, result); err != nil {
							slog.Error("validate buildgate: unmarshal result failed", "job_id", jobID, "err", err)
						}
					}

					resp := contracts.BuildGateValidateResponse{
						JobID:  jobID,
						Status: job.Status, // No cast needed: contracts.BuildGateJobStatus is now an alias for store.BuildgateJobStatus
						Result: result,
					}

					w.Header().Set("Content-Type", "application/json")
					if job.Status == store.BuildgateJobStatusCompleted {
						w.WriteHeader(http.StatusOK)
					} else {
						w.WriteHeader(http.StatusOK) // Still 200, but status indicates failure
					}

					if err := json.NewEncoder(w).Encode(resp); err != nil {
						slog.Error("validate buildgate: encode sync response failed", "err", err)
					}
					slog.Info("buildgate job completed (sync)", "job_id", jobID, "status", job.Status)
					return
				}
			}
		}
	}
}

// getBuildGateJobStatusHandler returns an HTTP handler for GET /v1/buildgate/jobs/{id}.
// Returns the current status and result of a build gate job.
func getBuildGateJobStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse job ID from URL path parameter.
		jobIDStr := r.PathValue("id")
		if jobIDStr == "" {
			jobIDStr = r.PathValue("job_id")
		}
		if jobIDStr == "" {
			http.Error(w, "job id is required", http.StatusBadRequest)
			return
		}

		// Parse UUID.
		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid job id: %v", err), http.StatusBadRequest)
			return
		}

		// Convert to pgtype.UUID.
		pgID := pgtype.UUID{
			Bytes: jobID,
			Valid: true,
		}

		// Fetch job.
		job, err := st.GetBuildGateJob(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("get buildgate job: fetch failed", "job_id", jobIDStr, "err", err)
			return
		}

		// Build response.
		var result *contracts.BuildGateStageMetadata
		if len(job.Result) > 0 {
			result = &contracts.BuildGateStageMetadata{}
			if err := json.Unmarshal(job.Result, result); err != nil {
				slog.Error("get buildgate job: unmarshal result failed", "job_id", jobIDStr, "err", err)
			}
		}

		errorMsg := ""
		if job.Error != nil {
			errorMsg = *job.Error
		}

		resp := contracts.BuildGateJobStatusResponse{
			JobID:      jobIDStr,
			Status:     job.Status, // No cast needed: contracts.BuildGateJobStatus is now an alias for store.BuildgateJobStatus
			Result:     result,
			Error:      errorMsg,
			CreatedAt:  formatTimestamp(job.CreatedAt),
			StartedAt:  formatOptionalTimestamp(job.StartedAt),
			FinishedAt: formatOptionalTimestamp(job.FinishedAt),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get buildgate job: encode response failed", "err", err)
		}
	}
}

// formatTimestamp converts pgtype.Timestamptz to ISO 8601 string.
func formatTimestamp(ts pgtype.Timestamptz) string {
	if ts.Valid {
		return ts.Time.UTC().Format(time.RFC3339)
	}
	return ""
}

// formatOptionalTimestamp converts optional pgtype.Timestamptz to pointer string.
func formatOptionalTimestamp(ts pgtype.Timestamptz) *string {
	if ts.Valid {
		s := ts.Time.UTC().Format(time.RFC3339)
		return &s
	}
	return nil
}

// claimBuildGateJobHandler returns an HTTP handler for POST /v1/nodes/{id}/buildgate/claim.
// Allows worker nodes to claim pending buildgate jobs.
func claimBuildGateJobHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse node ID from URL path parameter.
		nodeIDStr := r.PathValue("id")
		if nodeIDStr == "" {
			http.Error(w, "node id is required", http.StatusBadRequest)
			return
		}

		// Parse UUID.
		nodeID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid node id: %v", err), http.StatusBadRequest)
			return
		}

		// Convert to pgtype.UUID.
		pgNodeID := pgtype.UUID{
			Bytes: nodeID,
			Valid: true,
		}

		// Claim a buildgate job.
		job, err := st.ClaimBuildGateJob(r.Context(), pgNodeID)
		if err != nil {
			// Check if it's "no rows" (no work available).
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim job: %v", err), http.StatusInternalServerError)
			slog.Error("claim buildgate job: failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Parse request payload.
		var req contracts.BuildGateValidateRequest
		if err := json.Unmarshal(job.RequestPayload, &req); err != nil {
			http.Error(w, fmt.Sprintf("failed to parse request payload: %v", err), http.StatusInternalServerError)
			slog.Error("claim buildgate job: unmarshal payload failed", "job_id", uuid.UUID(job.ID.Bytes).String(), "err", err)
			return
		}

		// Build response.
		resp := struct {
			JobID   string                             `json:"job_id"`
			Request contracts.BuildGateValidateRequest `json:"request"`
			Status  store.BuildgateJobStatus           `json:"status"` // Use typed status instead of string cast
		}{
			JobID:   uuid.UUID(job.ID.Bytes).String(),
			Request: req,
			Status:  job.Status,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("claim buildgate job: encode response failed", "err", err)
		}

		slog.Info("buildgate job claimed", "job_id", resp.JobID, "node_id", nodeIDStr)
	}
}

// completeBuildGateJobHandler returns an HTTP handler for POST /v1/nodes/{id}/buildgate/{job_id}/complete.
// Allows worker nodes to mark buildgate jobs as complete.
func completeBuildGateJobHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse node ID and job ID from URL path parameters.
		nodeIDStr := r.PathValue("id")
		jobIDStr := r.PathValue("job_id")
		if nodeIDStr == "" || jobIDStr == "" {
			http.Error(w, "node id and job id are required", http.StatusBadRequest)
			return
		}

		// Parse job UUID.
		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid job id: %v", err), http.StatusBadRequest)
			return
		}

		// Parse request body.
		var reqBody struct {
			Status string                            `json:"status"` // "completed" or "failed"
			Result *contracts.BuildGateStageMetadata `json:"result,omitempty"`
			Error  *string                           `json:"error,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate status.
		var status store.BuildgateJobStatus
		switch reqBody.Status {
		case "completed":
			status = store.BuildgateJobStatusCompleted
		case "failed":
			status = store.BuildgateJobStatusFailed
		default:
			http.Error(w, "status must be 'completed' or 'failed'", http.StatusBadRequest)
			return
		}

		// Serialize result.
		var resultBytes []byte
		if reqBody.Result != nil {
			resultBytes, err = json.Marshal(reqBody.Result)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to serialize result: %v", err), http.StatusInternalServerError)
				slog.Error("complete buildgate job: marshal result failed", "job_id", jobIDStr, "err", err)
				return
			}
		}

		// Update job completion.
		pgJobID := pgtype.UUID{
			Bytes: jobID,
			Valid: true,
		}

		err = st.UpdateBuildGateJobCompletion(r.Context(), store.UpdateBuildGateJobCompletionParams{
			ID:     pgJobID,
			Status: status,
			Result: resultBytes,
			Error:  reqBody.Error,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update job: %v", err), http.StatusInternalServerError)
			slog.Error("complete buildgate job: update failed", "job_id", jobIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("buildgate job completed", "job_id", jobIDStr, "status", reqBody.Status)
	}
}

// ackBuildGateJobStartHandler returns an HTTP handler for
// POST /v1/nodes/{id}/buildgate/{job_id}/ack which transitions a claimed
// job into running state.
func ackBuildGateJobStartHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobIDStr := r.PathValue("job_id")
		if jobIDStr == "" {
			http.Error(w, "job id is required", http.StatusBadRequest)
			return
		}

		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid job id: %v", err), http.StatusBadRequest)
			return
		}

		pgJobID := pgtype.UUID{Bytes: jobID, Valid: true}
		if err := st.AckBuildGateJobStart(r.Context(), pgJobID); err != nil {
			http.Error(w, fmt.Sprintf("failed to ack job start: %v", err), http.StatusInternalServerError)
			slog.Error("ack buildgate job: update failed", "job_id", jobIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("buildgate job running", "job_id", jobIDStr)
	}
}
