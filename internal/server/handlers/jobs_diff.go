package handlers

import (
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

// createJobDiffHandler stores gzipped diff in object storage and metadata in diffs table (≤10 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/diff
func createJobDiffHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	if bp == nil {
		panic("createJobDiffHandler: blobpersist is required")
	}
	// Accept up to 16 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 10 MiB cap on the decoded patch bytes.
	const maxBodySize = 16 << 20  // 16 MiB
	const maxPatchSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		jobID, err := domaintypes.ParseJobIDParam(r, "job_id")
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
		var req struct {
			Patch   []byte                  `json:"patch"`   // gzipped diff (raw bytes)
			Summary domaintypes.DiffSummary `json:"summary"` // optional summary metadata
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
			return
		}

		// Validate patch is present.
		if len(req.Patch) == 0 {
			httpErr(w, http.StatusBadRequest, "patch is required")
			return
		}

		// Enforce decoded patch size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Patch) > maxPatchSize {
			httpErr(w, http.StatusRequestEntityTooLarge, "diff size exceeds 10 MiB cap")
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
			slog.Error("diff: run check failed", "run_id", runID.String(), "err", err)
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
			slog.Error("diff: job check failed", "job_id", jobID.String(), "err", err)
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

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "failed to marshal summary: %v", err)
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
			httpErr(w, http.StatusInternalServerError, "failed to create diff: %v", err)
			slog.Error("diff: create failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
			return
		}

		// Return success response with diff_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(struct {
			DiffID domaintypes.DiffID `json:"diff_id"`
		}{
			DiffID: domaintypes.DiffID(uuid.UUID(diff.ID.Bytes).String()),
		}); err != nil {
			slog.Error("diff: encode response failed", "err", err)
		}

		slog.Debug("diff created",
			"run_id", runID.String(),
			"job_id", jobID.String(),
			"diff_id", diff.ID.Bytes,
		)
	}
}
