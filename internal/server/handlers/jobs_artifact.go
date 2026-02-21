package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// nodeUUIDHeader is the HTTP header key that carries the worker node's ID
// (NanoID 6-character string) on mutating requests to the control plane.
const nodeUUIDHeader = "PLOY_NODE_UUID"

// computeArtifactCIDAndDigest computes a content identifier and SHA256 digest for an artifact bundle.
// CID uses a simple "bafy" prefix with hex-encoded SHA256 for compatibility with existing test fixtures.
// Digest is the full SHA256 hex string with "sha256:" prefix.
func computeArtifactCIDAndDigest(bundle []byte) (cid, digest string) {
	hash := sha256.Sum256(bundle)
	hexHash := hex.EncodeToString(hash[:])
	// Use bafy prefix (like IPFS CIDv1) followed by first 32 chars of hash for readability
	cid = "bafy" + hexHash[:32]
	digest = "sha256:" + hexHash
	return cid, digest
}

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
		runID, err := ParseRunIDParam(r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		jobID, err := ParseJobIDParam(r, "job_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			httpErr(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
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
			httpErr(w, http.StatusBadRequest, "bundle is required")
			return
		}

		// Enforce decoded bundle size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Bundle) > maxBundleSize {
			httpErr(w, http.StatusRequestEntityTooLarge, "artifact bundle size exceeds 10 MiB cap")
			return
		}

		// Check if the run exists.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check run: %v", err)
			slog.Error("artifact: run check failed", "run_id", runID.String(), "err", err)
			return
		}

		// Check if the job exists.
		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "job not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check job: %v", err)
			slog.Error("artifact: job check failed", "job_id", jobID.String(), "err", err)
			return
		}

		// Ensure the job belongs to the provided run.
		if job.RunID != runID {
			httpErr(w, http.StatusBadRequest, "job does not belong to run")
			return
		}

		// Verify the job is assigned to the calling node using the
		// PLOY_NODE_UUID header, which is required for worker requests.
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
		if job.NodeID == nil || *job.NodeID != nodeIDHeader {
			httpErr(w, http.StatusForbidden, "job not assigned to this node")
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
			httpErr(w, http.StatusInternalServerError, "failed to create artifact bundle: %v", err)
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
