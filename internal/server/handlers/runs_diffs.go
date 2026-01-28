package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// createRunDiffHandler stores a gzipped diff for a run using an optional job-scoped association.
func createRunDiffHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	if bp == nil {
		panic("createRunDiffHandler: blobpersist is required")
	}
	// Accept up to 16 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 10 MiB cap on the decoded patch bytes.
	const maxBodySize = 16 << 20  // 16 MiB
	const maxPatchSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		var req struct {
			JobID   *domaintypes.JobID      `json:"job_id,omitempty"`
			Patch   []byte                  `json:"patch"`
			Summary domaintypes.DiffSummary `json:"summary"`
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
			return
		}
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 10 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Validate job belongs to run if provided.
		var jobID *domaintypes.JobID
		if req.JobID != nil && !req.JobID.IsZero() {
			job, err := st.GetJob(r.Context(), *req.JobID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "job not found", http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed to check job: %v", err), http.StatusInternalServerError)
				slog.Error("run diff: job check failed", "job_id", req.JobID.String(), "err", err)
				return
			}
			if job.RunID != runID {
				http.Error(w, "job does not belong to run", http.StatusBadRequest)
				return
			}
			jobID = &job.ID
		}

		// Ensure the run exists.
		if _, err := st.GetRun(r.Context(), runID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("run diff: run check failed", "run_id", runID.String(), "err", err)
			return
		}

		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
			return
		}
		params := store.CreateDiffParams{
			RunID:   runID,
			JobID:   jobID,
			Summary: summaryBytes,
		}

		// Persist diff metadata to database and upload blob to object storage.
		diff, err := bp.CreateDiff(r.Context(), params, req.Patch)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
			slog.Error("run diff: create failed", "run_id", runID.String(), "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(struct {
			DiffID domaintypes.DiffID `json:"diff_id"`
		}{
			DiffID: domaintypes.DiffID(uuid.UUID(diff.ID.Bytes).String()),
		}); err != nil {
			slog.Error("run diff: encode response failed", "err", err)
		}
	}
}
