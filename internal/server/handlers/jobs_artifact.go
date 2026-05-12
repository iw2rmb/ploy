package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// nodeUUIDHeader is the HTTP header key that carries the worker node's ID
// (NanoID 6-character string) on mutating requests to the control plane.
const nodeUUIDHeader = "PLOY_NODE_UUID"

// parseNodeUUIDHeader extracts and validates the PLOY_NODE_UUID header value
// without writing a response. Used by call sites that compose validation in a
// larger function returning an error.
func parseNodeUUIDHeader(r *http.Request) (domaintypes.NodeID, error) {
	headerVal := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
	if headerVal == "" {
		return "", errors.New("PLOY_NODE_UUID header is required")
	}
	var nodeID domaintypes.NodeID
	if err := nodeID.UnmarshalText([]byte(headerVal)); err != nil {
		return "", errors.New("invalid PLOY_NODE_UUID header")
	}
	return nodeID, nil
}

// requireNodeUUIDHeader validates the PLOY_NODE_UUID header.
// On failure it writes a 400 response and returns ok=false.
func requireNodeUUIDHeader(w http.ResponseWriter, r *http.Request) (domaintypes.NodeID, bool) {
	nodeID, err := parseNodeUUIDHeader(r)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "%s", err)
		return "", false
	}
	return nodeID, true
}

// assertJobAssignedToNode enforces that the job is assigned to the given node.
// On mismatch it writes a 403 response and returns ok=false.
func assertJobAssignedToNode(w http.ResponseWriter, job store.Job, nodeID domaintypes.NodeID) bool {
	if job.NodeID == nil || *job.NodeID != nodeID {
		writeHTTPError(w, http.StatusForbidden, "job not assigned to this node")
		return false
	}
	return true
}

// createJobArtifactHandler stores gzipped artifact bundle in object storage and metadata in artifact_bundles table (≤10 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/artifact
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
// Note: build_id removed as part of builds table removal; artifacts now use job-level grouping only.
func createJobArtifactHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	requireBlobPersist("createJobArtifactHandler", bp)
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}

		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		// Check payload size before reading body.
		if rejectOversizedContentLength(w, r, ingestMaxBodySize) {
			return
		}

		// Decode request body with strict validation.
		// Note: build_id removed; artifacts are now grouped at job level only.
		var req struct {
			Name   *string `json:"name"`   // optional logical name
			Bundle []byte  `json:"bundle"` // gzipped tar (raw bytes)
		}

		if err := decodeRequestJSON(w, r, &req, ingestMaxBodySize); err != nil {
			return
		}

		// Validate bundle is present.
		if len(req.Bundle) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "bundle is required")
			return
		}

		// Enforce decoded bundle size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Bundle) > ingestMaxDataSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "artifact bundle size exceeds 10 MiB cap")
			return
		}

		job, ok := getJobInRunOrFail(w, r, st, runID, jobID, "artifact")
		if !ok {
			return
		}

		nodeIDHeader, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}
		if !assertJobAssignedToNode(w, job, nodeIDHeader) {
			return
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeCIDAndDigest(req.Bundle)

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
			writeHTTPError(w, http.StatusInternalServerError, "failed to create artifact bundle: %v", err)
			slog.Error("artifact: create failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String(),
			"cid":                strings.TrimSpace(*artifact.Cid),
			"digest":             strings.TrimSpace(*artifact.Digest),
		})

		slog.Debug("artifact bundle created",
			"run_id", runID.String(),
			"job_id", jobID.String(),
			"artifact_bundle_id", artifact.ID.Bytes,
		)
	}
}
