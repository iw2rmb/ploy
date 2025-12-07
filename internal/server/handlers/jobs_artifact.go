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

	"github.com/iw2rmb/ploy/internal/store"
)

// createJobArtifactHandler stores gzipped artifact bundle in artifact_bundles table (≤1 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/artifact
//
// Run and job IDs are now KSUID-backed strings; no UUID parsing is performed.
func createJobArtifactHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded bundle bytes.
	const maxBodySize = 2 << 20   // 2 MiB
	const maxBundleSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run_id from path parameter (KSUID string).
		runIDStr := strings.TrimSpace(r.PathValue("run_id"))
		if runIDStr == "" {
			http.Error(w, "run_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Extract job_id from path parameter (KSUID string).
		jobIDStr := strings.TrimSpace(r.PathValue("job_id"))
		if jobIDStr == "" {
			http.Error(w, "job_id path parameter is required", http.StatusBadRequest)
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
			BuildID *string `json:"build_id"` // optional (KSUID string)
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

		// Check if the run exists using string ID directly.
		var err error
		_, err = st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Check if the job exists using string ID directly.
		job, err := st.GetJob(r.Context(), jobIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check job: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: job check failed", "job_id", jobIDStr, "err", err)
			return
		}

		// Ensure the job belongs to the provided run (both are strings now).
		if job.RunID != runIDStr {
			http.Error(w, "job does not belong to run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node using the
		// PLOY_NODE_UUID header, which is required for worker requests.
		// Node IDs are now NanoID(6) strings; validate non-empty and match job assignment.
		nodeIDHeader := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeader == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}
		// job.NodeID is *string after node ID migration.
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Normalize optional build_id (KSUID string).
		buildID := normalizeOptionalID(req.BuildID)

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)

		// Create artifact bundle params using string IDs.
		params := store.CreateArtifactBundleParams{
			RunID:   runIDStr,
			JobID:   &jobIDStr,
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
		// artifact_bundles.id is still UUID.
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
