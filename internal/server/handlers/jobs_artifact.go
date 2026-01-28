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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// createJobArtifactHandler stores gzipped artifact bundle in object storage and metadata in artifact_bundles table (≤10 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/artifact
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
// Note: build_id removed as part of builds table removal; artifacts now use job-level grouping only.
func createJobArtifactHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	if bp == nil {
		panic("createJobArtifactHandler: blobpersist is required")
	}
	// Accept up to 16 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 10 MiB cap on the decoded bundle bytes.
	const maxBodySize = 16 << 20   // 16 MiB
	const maxBundleSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "run_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		jobID, err := domaintypes.ParseJobIDParam(r, "job_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Decode request body with strict validation.
		// Note: build_id removed; artifacts are now grouped at job level only.
		var req struct {
			Name   *string `json:"name"`   // optional logical name
			Bundle []byte  `json:"bundle"` // gzipped tar (raw bytes)
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
			return
		}

		// Validate bundle is present.
		if len(req.Bundle) == 0 {
			http.Error(w, "bundle is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded bundle size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Bundle) > maxBundleSize {
			http.Error(w, "artifact bundle size exceeds 10 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the run exists.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: run check failed", "run_id", runID.String(), "err", err)
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
			slog.Error("artifact: job check failed", "job_id", jobID.String(), "err", err)
			return
		}

		// Ensure the job belongs to the provided run.
		if job.RunID != runID {
			http.Error(w, "job does not belong to run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node using the
		// PLOY_NODE_UUID header, which is required for worker requests.
		nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
		if nodeIDHeaderStr == "" {
			http.Error(w, "PLOY_NODE_UUID header is required", http.StatusBadRequest)
			return
		}
		var nodeIDHeader domaintypes.NodeID
		if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
			http.Error(w, "invalid PLOY_NODE_UUID header", http.StatusBadRequest)
			return
		}
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)

		// Create artifact bundle params using domain RunID.
		params := store.CreateArtifactBundleParams{
			RunID:  runID,
			JobID:  &jobID,
			Name:   req.Name,
			Cid:    &cid,
			Digest: &digest,
		}

		// Persist artifact bundle metadata to database and upload blob to object storage.
		artifact, err := bp.CreateArtifactBundle(r.Context(), params, req.Bundle)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create artifact bundle: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: create failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
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
			"run_id", runID.String(),
			"job_id", jobID.String(),
			"artifact_bundle_id", artifact.ID.Bytes,
		)
	}
}
