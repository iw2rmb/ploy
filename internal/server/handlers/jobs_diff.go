package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// createJobDiffHandler stores gzipped diff in object storage and metadata in diffs table (≤10 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/diff
func createJobDiffHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	requireBlobPersist("createJobDiffHandler", bp)
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
		var req struct {
			Patch   []byte                  `json:"patch"`   // gzipped diff (raw bytes)
			Summary domaintypes.DiffSummary `json:"summary"` // optional summary metadata
		}

		if err := decodeRequestJSON(w, r, &req, ingestMaxBodySize); err != nil {
			return
		}

		// Validate patch is present.
		if len(req.Patch) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "patch is required")
			return
		}

		// Enforce decoded patch size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Patch) > ingestMaxDataSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "diff size exceeds 10 MiB cap")
			return
		}

		job, ok := getJobInRunOrFail(w, r, st, runID, jobID, "diff")
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

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "failed to marshal summary: %v", err)
			return
		}

		// Store params use domain RunID (KSUID-backed) and string job ID.
		params := store.CreateDiffParams{
			RunID:   runID,
			JobID:   &jobID,
			Summary: summaryBytes,
		}

		// Persist diff metadata to database and upload blob to object storage.
		diff, err := bp.CreateDiff(r.Context(), params, req.Patch)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create diff: %v", err)
			slog.Error("diff: create failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
			return
		}

		writeJSON(w, http.StatusCreated, struct {
			DiffID domaintypes.DiffID `json:"diff_id"`
		}{
			DiffID: domaintypes.DiffID(uuid.UUID(diff.ID.Bytes).String()),
		})

		slog.Debug("diff created",
			"run_id", runID.String(),
			"job_id", jobID.String(),
			"diff_id", diff.ID.Bytes,
		)
	}
}
