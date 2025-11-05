package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// createRunLogHandler handles POST /v1/runs/{id}/logs for receiving gzipped log chunks.
// This variant does not require a node path parameter; it ingests logs scoped to a run.
func createRunLogHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxChunkSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter.
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			StageID *string `json:"stage_id,omitempty"`
			BuildID *string `json:"build_id,omitempty"`
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

		// Parse optional stage/build IDs using helper.
		stageID, err := parseOptionalUUID(req.StageID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid stage_id: %v", err), http.StatusBadRequest)
			return
		}
		buildID, err := parseOptionalUUID(req.BuildID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid build_id: %v", err), http.StatusBadRequest)
			return
		}

		// Create log row.
		params := store.CreateLogParams{
			RunID:   pgtype.UUID{Bytes: runUUID, Valid: true},
			StageID: stageID,
			BuildID: buildID,
			ChunkNo: req.ChunkNo,
			Data:    req.Data,
		}
		// Persist and publish to SSE when events service is available; otherwise
		// fall back to direct store write for backward compatibility.
		var logRow store.Log
		if eventsService != nil {
			logRow, err = eventsService.CreateAndPublishLog(r.Context(), params)
		} else {
			logRow, err = st.CreateLog(r.Context(), params)
		}
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
