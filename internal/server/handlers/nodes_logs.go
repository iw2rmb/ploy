package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
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
// The blobpersist service handles database metadata and object storage writes.
// The events service handles SSE fanout.
func createNodeLogsHandler(st store.Store, bp *blobpersist.Service, eventsService *events.Service) http.HandlerFunc {
	// Validate dependencies are provided.
	if bp == nil {
		panic("createNodeLogsHandler: blobpersist is required")
	}
	if eventsService == nil {
		panic("createNodeLogsHandler: eventsService is required")
	}
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxChunkSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := domaintypes.ParseNodeIDParam(r, "id")
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
		// Note: build_id removed; logs are now grouped at job level only.
		// Uses domain types (RunID, JobID) for type-safe request parsing.
		var req struct {
			RunID   domaintypes.RunID  `json:"run_id"`           // Run ID (KSUID-backed)
			JobID   *domaintypes.JobID `json:"job_id,omitempty"` // Job ID (KSUID-backed, optional)
			ChunkNo int32              `json:"chunk_no"`
			Data    []byte             `json:"data"`
		}

		if err := DecodeJSON(w, r, &req, maxBodySize); err != nil {
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
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: check failed", "node_id", nodeID.String(), "err", err)
			return
		}

		// Parse job_id if provided.
		var jobID *domaintypes.JobID
		if req.JobID != nil && !req.JobID.IsZero() {
			jobID = req.JobID
		}

		// Store the gzipped log chunk in the database and object storage.
		params := store.CreateLogParams{
			RunID:   req.RunID,
			JobID:   jobID,
			ChunkNo: req.ChunkNo,
		}

		// Persist log metadata to database and upload blob to object storage.
		log, err := bp.CreateLog(r.Context(), params, req.Data)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create log: %v", err), http.StatusInternalServerError)
			slog.Error("node logs: create failed", "node_id", nodeID.String(), "run_id", req.RunID.String(), "chunk_no", req.ChunkNo, "err", err)
			return
		}

		// Publish to SSE hub for real-time streaming.
		if err := eventsService.CreateAndPublishLog(r.Context(), log, req.Data); err != nil {
			// Log the error but don't fail the operation since DB/blob write succeeded.
			slog.Error("node logs: SSE fanout failed", "log_id", log.ID, "err", err)
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
			"node_id", nodeID.String(),
			"run_id", req.RunID.String(),
			"chunk_no", req.ChunkNo,
			"size", len(req.Data),
		)
	}
}
