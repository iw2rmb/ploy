package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// createRunLogHandler handles POST /v1/runs/{id}/logs for receiving gzipped log chunks.
// This variant does not require a node path parameter; it ingests logs scoped to a run.
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
// Note: build_id removed as part of builds table removal; logs now use job-level grouping only.
//
// The blobpersist service handles database metadata and object storage writes.
// The events service handles SSE fanout.
func createRunLogHandler(st store.Store, bp *blobpersist.Service, eventsService *server.EventsService) http.HandlerFunc {
	// Validate dependencies are provided.
	if bp == nil {
		panic("createRunLogHandler: blobpersist is required")
	}
	if eventsService == nil {
		panic("createRunLogHandler: eventsService is required")
	}
	// Accept up to 16 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 10 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 16 << 20  // 16 MiB
	const maxChunkSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter using domain type helper.
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return
		}

		// Decode request body with strict validation.
		// Note: build_id removed; logs are now grouped at job level only.
		var req struct {
			JobID   *domaintypes.JobID `json:"job_id,omitempty"`
			ChunkNo int32              `json:"chunk_no"`
			Data    []byte             `json:"data"`
		}

		if err := decodeRequestJSON(w, r, &req, maxBodySize); err != nil {
			return
		}

		if len(req.Data) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "data is required and must not be empty")
			return
		}
		if len(req.Data) > maxChunkSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "data exceeds 10 MiB: %d bytes", len(req.Data))
			return
		}

		// Create log row using string run ID and string job ID.
		params := store.CreateLogParams{
			RunID:   runID,
			JobID:   req.JobID,
			ChunkNo: req.ChunkNo,
		}

		// Persist log metadata to database and upload blob to object storage.
		logRow, err := bp.CreateLog(r.Context(), params, req.Data)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create log: %v", err)
			slog.Error("run logs: create failed", "run_id", runID, "chunk_no", req.ChunkNo, "err", err)
			return
		}

		// Publish to SSE hub for real-time streaming.
		if err := eventsService.CreateAndPublishLog(r.Context(), logRow, req.Data); err != nil {
			// Log the error but don't fail the operation since DB/blob write succeeded.
			slog.Error("run logs: SSE fanout failed", "log_id", logRow.ID, "err", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"id": logRow.ID, "chunk_no": logRow.ChunkNo}); err != nil {
			slog.Error("run logs: encode response failed", "err", err)
		}
	}
}
