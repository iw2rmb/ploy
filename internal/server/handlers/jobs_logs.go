package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// getJobLogsHandler returns an HTTP handler that streams job logs over SSE.
// GET /v1/jobs/{job_id}/logs — SSE for job-scoped container logs.
//
// Only log and done event types are emitted on the job-keyed stream.
// Supports Last-Event-ID header for resuming from a specific cursor.
//
// For fresh connections (sinceID == 0), the handler backfills historical logs
// from the database and object store, then subscribes to the job stream.
// For terminal jobs, the handler writes a done sentinel after backfill.
func getJobLogsHandler(st store.Store, bs blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := parseRequiredPathID[domaintypes.JobID](r, "job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "job not found")
				return
			}
			slog.Error("get job logs: database error", "job_id", jobID.String(), "err", err)
			writeHTTPError(w, http.StatusInternalServerError, "failed to get job")
			return
		}

		_, ok := getRunOrFail(w, r, st, job.RunID, "get job logs")
		if !ok {
			return
		}

		hub := eventsService.Hub()
		if err := hub.EnsureJob(jobID); err != nil {
			slog.Error("ensure job stream failed", "job_id", jobID.String(), "err", err)
			writeHTTPError(w, http.StatusBadRequest, "invalid job id")
			return
		}

		allowedJobs := map[domaintypes.JobID]struct{}{jobID: {}}
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// For fresh connections, backfill historical logs then subscribe to job stream.
		if sinceID == 0 && bs != nil {
			if serveJobWithBackfill(w, r, st, bs, hub, job, allowedJobs) {
				return
			}
		}

		// Non-zero sinceID or backfill setup failed: subscribe directly to job stream.
		if err := logstream.ServeJob(w, r, hub, jobID, sinceID); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream job logs", "job_id", jobID.String(), "err", err)
			}
		}
	}
}

// createJobLogsHandler handles POST /v1/jobs/{job_id}/logs for receiving gzipped
// log chunks scoped to a specific job. Resolves run context from the job row and
// reuses the same chunk validation and persistence semantics as other log endpoints.
func createJobLogsHandler(st store.Store, bp *blobpersist.Service, eventsService *server.EventsService) http.HandlerFunc {
	if bp == nil {
		panic("createJobLogsHandler: blobpersist is required")
	}
	if eventsService == nil {
		panic("createJobLogsHandler: eventsService is required")
	}
	const maxBodySize = 16 << 20  // 16 MiB
	const maxChunkSize = 10 << 20 // 10 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := parseRequiredPathID[domaintypes.JobID](r, "job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve job to get run context.
		job, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "job not found")
				return
			}
			slog.Error("job logs ingest: get job failed", "job_id", jobID.String(), "err", err)
			writeHTTPError(w, http.StatusInternalServerError, "failed to get job")
			return
		}

		if r.ContentLength > maxBodySize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return
		}

		var req struct {
			ChunkNo int32  `json:"chunk_no"`
			Data    []byte `json:"data"`
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

		params := store.CreateLogParams{
			RunID:   job.RunID,
			JobID:   &jobID,
			ChunkNo: req.ChunkNo,
		}

		logRow, err := bp.CreateLog(r.Context(), params, req.Data)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create log: %v", err)
			slog.Error("job logs ingest: create failed", "job_id", jobID.String(), "chunk_no", req.ChunkNo, "err", err)
			return
		}

		if err := eventsService.CreateAndPublishLog(r.Context(), logRow, req.Data); err != nil {
			slog.Error("job logs ingest: SSE fanout failed", "log_id", logRow.ID, "err", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"id": logRow.ID, "chunk_no": logRow.ChunkNo}); err != nil {
			slog.Error("job logs ingest: encode response failed", "err", err)
		}
	}
}

// buildJobLogFilter returns a filter that passes only log events for the given jobs
// and done events. Run lifecycle events (run, stage, retention) are excluded.
func buildJobLogFilter(allowedJobs map[domaintypes.JobID]struct{}) func(logstream.Event) (logstream.Event, bool) {
	return func(evt logstream.Event) (logstream.Event, bool) {
		switch evt.Type {
		case domaintypes.SSEEventLog:
			var payload struct {
				JobID *string `json:"job_id,omitempty"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return evt, false
			}
			if payload.JobID == nil {
				return evt, false
			}
			var jobID domaintypes.JobID
			if err := jobID.UnmarshalText([]byte(*payload.JobID)); err != nil {
				return evt, false
			}
			_, ok := allowedJobs[jobID]
			return evt, ok
		case domaintypes.SSEEventDone:
			return evt, true
		default:
			// Exclude run, stage, retention from job-scoped stream.
			return evt, false
		}
	}
}
