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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// createNodeLogsHandler handles POST /v1/nodes/{id}/logs for receiving gzipped log chunks.
func createNodeLogsHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxChunkSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body but allow base64 overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			RunID   string  `json:"run_id"`
			StageID *string `json:"stage_id,omitempty"`
			BuildID *string `json:"build_id,omitempty"`
			ChunkNo int32   `json:"chunk_no"`
			Data    []byte  `json:"data"`
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

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate run_id is a valid UUID.
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate data is not empty.
		if len(req.Data) == 0 {
			http.Error(w, "data is required and must not be empty", http.StatusBadRequest)
			return
		}

		// Enforce 1 MiB cap on decoded gzipped data bytes.
		if len(req.Data) > maxChunkSize {
			http.Error(w, fmt.Sprintf("data exceeds 1 MiB: %d bytes", len(req.Data)), http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Parse stage_id if provided.
		var stageID pgtype.UUID
		if req.StageID != nil && strings.TrimSpace(*req.StageID) != "" {
			stageUUID, err := uuid.Parse(*req.StageID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid stage_id: %v", err), http.StatusBadRequest)
				return
			}
			stageID = pgtype.UUID{
				Bytes: stageUUID,
				Valid: true,
			}
		}

		// Parse build_id if provided.
		var buildID pgtype.UUID
		if req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "" {
			buildUUID, err := uuid.Parse(*req.BuildID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid build_id: %v", err), http.StatusBadRequest)
				return
			}
			buildID = pgtype.UUID{
				Bytes: buildUUID,
				Valid: true,
			}
		}

		// Store the gzipped log chunk in the database.
		params := store.CreateLogParams{
			RunID: pgtype.UUID{
				Bytes: runUUID,
				Valid: true,
			},
			StageID: stageID,
			BuildID: buildID,
			ChunkNo: req.ChunkNo,
			Data:    req.Data,
		}

		log, err := st.CreateLog(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create log: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "chunk_no", req.ChunkNo, "err", err)
			return
		}

		// Return success response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"id":       log.ID,
			"chunk_no": log.ChunkNo,
		}); err != nil {
			slog.Error("node logs: encode response failed", "err", err)
		}

		slog.Debug("log chunk stored",
			"node_id", nodeIDStr,
			"run_id", req.RunID,
			"chunk_no", req.ChunkNo,
			"size", len(req.Data),
		)
	}
}
