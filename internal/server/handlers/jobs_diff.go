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
		var req struct {
			Patch   []byte                  `json:"patch"`   // gzipped diff (raw bytes)
			Summary domaintypes.DiffSummary `json:"summary"` // optional summary metadata
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
			return
		}

		// Validate patch is present.
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded patch size cap (≤ 10 MiB gzipped, base64-decoded here).
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 10 MiB cap", http.StatusRequestEntityTooLarge)
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
			slog.Error("diff: run check failed", "run_id", runID.String(), "err", err)
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
			slog.Error("diff: job check failed", "job_id", jobID.String(), "err", err)
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

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
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
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
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
