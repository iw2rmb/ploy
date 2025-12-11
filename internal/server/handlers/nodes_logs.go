package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// nodeLogCreateResponse is the response for POST /v1/nodes/{id}/logs.
type nodeLogCreateResponse struct {
	ID      int64 `json:"id"`
	ChunkNo int32 `json:"chunk_no"`
}

// createNodeLogsHandler handles POST /v1/nodes/{id}/logs for receiving gzipped log chunks.
// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
//
// The eventsService parameter is required and must not be nil. Log ingestion always
// goes through the events service to ensure both database persistence and SSE fanout
// occur in a single path. Direct store writes are no longer supported.
func createNodeLogsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	// Validate eventsService is provided — log ingestion requires SSE fanout.
	if eventsService == nil {
		panic("createNodeLogsHandler: eventsService is required")
	}
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

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body but allow base64 overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		// Note: build_id removed; logs are now grouped at job level only.
		// Uses domain types (RunID, JobID) for type-safe request parsing.
		var req struct {
			RunID   domaintypes.RunID  `json:"run_id"`           // Run ID (KSUID-backed)
			JobID   *domaintypes.JobID `json:"job_id,omitempty"` // Job ID (KSUID-backed, optional)
			ChunkNo int32              `json:"chunk_no"`
			Data    []byte             `json:"data"`
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

		// Validate run_id is present using domain type's IsZero method.
		if req.RunID.IsZero() {
			http.Error(w, "run_id is required", http.StatusBadRequest)
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
		var err error
		_, err = st.GetNode(r.Context(), nodeIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Parse job_id if provided; convert domain type to string for store.
		var jobID *string
		if req.JobID != nil && !req.JobID.IsZero() {
			s := req.JobID.String()
			jobID = &s
		}

		// Store the gzipped log chunk in the database using domain RunID.
		params := store.CreateLogParams{
			RunID:   req.RunID,
			JobID:   jobID,
			ChunkNo: req.ChunkNo,
			Data:    req.Data,
		}

		// Persist log to database and publish to SSE hub via events service.
		// This is the single canonical logging path — no direct store fallback.
		log, err := eventsService.CreateAndPublishLog(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create log: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "chunk_no", req.ChunkNo, "err", err)
			return
		}

		// Return success response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := nodeLogCreateResponse{
			ID:      log.ID,
			ChunkNo: log.ChunkNo,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
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
