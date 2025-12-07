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

	"github.com/iw2rmb/ploy/internal/store"
)

// createRunArtifactBundleHandler stores a gzipped artifact bundle for a run using an optional job-scoped association.
//
// Run, job, and build IDs are now KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks and existence checks.
func createRunArtifactBundleHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded bundle bytes.
	const maxBodySize = 2 << 20   // 2 MiB
	const maxBundleSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		var req struct {
			JobID   *string `json:"job_id,omitempty"`
			BuildID *string `json:"build_id,omitempty"`
			Name    *string `json:"name"`
			Bundle  []byte  `json:"bundle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
		if len(req.Bundle) == 0 {
			http.Error(w, "bundle is required", http.StatusBadRequest)
			return
		}
		if len(req.Bundle) > maxBundleSize {
			http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Normalize optional job/build IDs (KSUID strings; no UUID parsing).
		jobID := normalizeOptionalID(req.JobID)
		buildID := normalizeOptionalID(req.BuildID)

		// Validate job belongs to run if provided.
		if jobID != nil {
			job, err := st.GetJob(r.Context(), *jobID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "job not found", http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed to check job: %v", err), http.StatusInternalServerError)
				slog.Error("run artifact: job check failed", "job_id", *jobID, "err", err)
				return
			}
			// Compare string run IDs directly (both are KSUID strings).
			if job.RunID != runIDStr {
				http.Error(w, "job does not belong to run", http.StatusBadRequest)
				return
			}
		}

		// Ensure the run exists using string ID directly.
		if _, err := st.GetRun(r.Context(), runIDStr); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("run artifact: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)
		params := store.CreateArtifactBundleParams{
			RunID:   runIDStr,
			JobID:   jobID,
			BuildID: buildID,
			Name:    req.Name,
			Bundle:  req.Bundle,
			Cid:     &cid,
			Digest:  &digest,
		}
		artifact, err := st.CreateArtifactBundle(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create artifact bundle: %v", err), http.StatusInternalServerError)
			slog.Error("run artifact: create failed", "run_id", runIDStr, "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// artifact_bundles.id is still UUID (not in scope of this task).
		if err := json.NewEncoder(w).Encode(map[string]any{"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String()}); err != nil {
			slog.Error("run artifact: encode response failed", "err", err)
		}
	}
}
