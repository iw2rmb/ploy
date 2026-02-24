package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
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
func createNodeLogsHandler(st store.Store, bp *blobpersist.Service, eventsService *server.EventsService) http.HandlerFunc {
	// Validate dependencies are provided.
	if bp == nil {
		panic("createNodeLogsHandler: blobpersist is required")
	}
	if eventsService == nil {
		panic("createNodeLogsHandler: eventsService is required")
	}
	// Accept up to 16 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 10 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 16 << 20  // 16 MiB
	const maxChunkSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := parseParam[domaintypes.NodeID](r, "id")
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
			httpErr(w, http.StatusBadRequest, "run_id is required")
			return
		}

		// Validate data is not empty.
		if len(req.Data) == 0 {
			httpErr(w, http.StatusBadRequest, "data is required and must not be empty")
			return
		}

		// Enforce 10 MiB cap on decoded gzipped data bytes.
		if len(req.Data) > maxChunkSize {
			httpErr(w, http.StatusRequestEntityTooLarge, "data exceeds 10 MiB: %d bytes", len(req.Data))
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check node: %v", err)
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
			httpErr(w, http.StatusInternalServerError, "failed to create log: %v", err)
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
