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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// createJobDiffHandler stores gzipped diff in diffs table (≤1 MiB), rejects oversize.
// Route: POST /v1/runs/{run_id}/jobs/{job_id}/diff
func createJobDiffHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded patch bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxPatchSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run_id from path parameter (KSUID-backed string).
		runIDStr := strings.TrimSpace(r.PathValue("run_id"))
		if runIDStr == "" {
			http.Error(w, "run_id path parameter is required", http.StatusBadRequest)
			return
		}

		// Extract job_id from path parameter (KSUID-backed string).
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
			Patch   []byte                  `json:"patch"`   // gzipped diff (raw bytes)
			Summary domaintypes.DiffSummary `json:"summary"` // optional summary metadata
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

		// Validate patch is present.
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded patch size cap (≤ 1 MiB gzipped, base64-decoded here).
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the run exists. Run IDs are KSUID strings.
		var err error
		_, err = st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("diff: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Check if the job exists. Job IDs are KSUID strings.
		job, err := st.GetJob(r.Context(), jobIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check job: %v", err), http.StatusInternalServerError)
			slog.Error("diff: job check failed", "job_id", jobIDStr, "err", err)
			return
		}

		// Ensure the job belongs to the provided run.
		if job.RunID.String() != runIDStr {
			http.Error(w, "job does not belong to run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the calling node using the
		// PLOY_NODE_UUID header, which is required for worker requests.
		// Node IDs are now NanoID(6) strings.
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

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
			return
		}

		// Store params use domain RunID (KSUID-backed) and string job ID.
		params := store.CreateDiffParams{
			RunID:   domaintypes.RunID(runIDStr),
			JobID:   &jobIDStr,
			Patch:   req.Patch,
			Summary: summaryBytes,
		}

		// Persist diff to DB.
		diff, err := st.CreateDiff(r.Context(), params)
		if err != nil {
			// Check if the error is a constraint violation (size cap exceeded).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
			slog.Error("diff: create failed", "run_id", runIDStr, "job_id", jobIDStr, "err", err)
			return
		}

		// Return success response with diff_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"diff_id": uuid.UUID(diff.ID.Bytes).String(),
		}); err != nil {
			slog.Error("diff: encode response failed", "err", err)
		}

		slog.Debug("diff created",
			"run_id", runIDStr,
			"job_id", jobIDStr,
			"diff_id", diff.ID.Bytes,
		)
	}
}
