package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// createJobArtifactHandler stores gzipped artifact bundle in artifact_bundles table (≤1 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/artifact
func createJobArtifactHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded bundle bytes.
	const maxBodySize = 2 << 20   // 2 MiB
	const maxBundleSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run_id from path parameter.
		runIDStr := r.PathValue("run_id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "run_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runID := domaintypes.ToPGUUID(runIDStr)
		if !runID.Valid {
			http.Error(w, "invalid run_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Extract job_id from path parameter.
		jobIDStr := r.PathValue("job_id")
		if strings.TrimSpace(jobIDStr) == "" {
			http.Error(w, "job_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate job_id.
		jobID := domaintypes.ToPGUUID(jobIDStr)
		if !jobID.Valid {
			http.Error(w, "invalid job_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body size to avoid memory exhaustion but allow base64 overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			BuildID *string `json:"build_id"` // optional
			Name    *string `json:"name"`     // optional logical name
			Bundle  []byte  `json:"bundle"`   // gzipped tar (raw bytes)
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Return 413 when MaxBytesReader trips the size cap.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate bundle is present.
		if len(req.Bundle) == 0 {
			http.Error(w, "bundle is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded bundle size cap (≤ 1 MiB gzipped, base64-decoded here).
		if len(req.Bundle) > maxBundleSize {
			http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the run exists.
		var err error
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Check if the job exists.
		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check job: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: job check failed", "job_id", jobIDStr, "err", err)
			return
		}

		// Ensure the job belongs to the provided run.
		if job.RunID != runID {
			http.Error(w, "job does not belong to run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node using the
		// PLOY_NODE_UUID header, which is required for worker requests.
		nodeIDHeader := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeader == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}
		nodeID := domaintypes.ToPGUUID(nodeIDHeader)
		if !nodeID.Valid {
			http.Error(w, "invalid PLOY_NODE_UUID header: invalid uuid", http.StatusBadRequest)
			return
		}
		if !job.NodeID.Valid || job.NodeID != nodeID {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Validate build_id if provided.
		var buildID pgtype.UUID
		if req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "" {
			buildID = domaintypes.ToPGUUID(*req.BuildID)
			if !buildID.Valid {
				http.Error(w, "invalid build_id: invalid uuid", http.StatusBadRequest)
				return
			}
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)

		// Create artifact bundle params.
		params := store.CreateArtifactBundleParams{
			RunID:   runID,
			JobID:   jobID,
			BuildID: buildID,
			Name:    req.Name,
			Bundle:  req.Bundle,
			Cid:     &cid,
			Digest:  &digest,
		}

		// Persist artifact bundle to DB.
		artifact, err := st.CreateArtifactBundle(r.Context(), params)
		if err != nil {
			// Check if the error is a constraint violation (size cap exceeded).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create artifact bundle: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: create failed", "run_id", runIDStr, "job_id", jobIDStr, "err", err)
			return
		}

		// Return success response with artifact_bundle_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String(),
			"cid":                strings.TrimSpace(*artifact.Cid),
			"digest":             strings.TrimSpace(*artifact.Digest),
		}); err != nil {
			slog.Error("artifact: encode response failed", "err", err)
		}

		slog.Debug("artifact bundle created",
			"run_id", runIDStr,
			"job_id", jobIDStr,
			"artifact_bundle_id", artifact.ID.Bytes,
		)
	}
}
