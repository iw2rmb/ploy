package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// createRunLogHandler handles POST /v1/runs/{id}/logs for receiving gzipped log chunks.
// This variant does not require a node path parameter; it ingests logs scoped to a run.
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
//
// The eventsService parameter is required and must not be nil. Log ingestion always
// goes through the events service to ensure both database persistence and SSE fanout
// occur in a single path. Direct store writes are no longer supported.
func createRunLogHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	// Validate eventsService is provided — log ingestion requires SSE fanout.
	if eventsService == nil {
		panic("createRunLogHandler: eventsService is required")
	}
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxChunkSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter using the shared helper.
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		// Note: build_id removed; logs are now grouped at job level only.
		var req struct {
			JobID   *string `json:"job_id,omitempty"`
			ChunkNo int32   `json:"chunk_no"`
			Data    []byte  `json:"data"`
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

		if len(req.Data) == 0 {
			http.Error(w, "data is required and must not be empty", http.StatusBadRequest)
			return
		}
		if len(req.Data) > maxChunkSize {
			http.Error(w, fmt.Sprintf("data exceeds 1 MiB: %d bytes", len(req.Data)), http.StatusRequestEntityTooLarge)
			return
		}

		// Normalize optional job ID (KSUID string; no UUID parsing).
		jobID := normalizeOptionalID(req.JobID)

		// Create log row using domain RunID and string job ID.
		params := store.CreateLogParams{
			RunID:   domaintypes.RunID(runIDStr),
			JobID:   jobID,
			ChunkNo: req.ChunkNo,
			Data:    req.Data,
		}
		// Persist log to database and publish to SSE hub via events service.
		// This is the single canonical logging path — no direct store fallback.
		logRow, err := eventsService.CreateAndPublishLog(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create log: %v", err), http.StatusInternalServerError)
			slog.Error("run logs: create failed", "run_id", runIDStr, "chunk_no", req.ChunkNo, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"id": logRow.ID, "chunk_no": logRow.ChunkNo}); err != nil {
			slog.Error("run logs: encode response failed", "err", err)
		}
	}
}
