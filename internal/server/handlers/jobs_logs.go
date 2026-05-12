package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

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
// Only log, retention, and done event types are emitted on the job-keyed stream.
// Supports Last-Event-ID header for resuming from a specific cursor.
//
// For fresh connections (sinceID == 0), the handler backfills historical logs
// from the database and object store, then subscribes to the job stream.
// For terminal jobs, the handler writes a done sentinel after backfill.
func getJobLogsHandler(st store.Store, bs blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		job, ok := getJobOrFail(w, r, st, jobID, "get job logs")
		if !ok {
			return
		}

		if _, ok := getRunOrFail(w, r, st, job.RunID, "get job logs"); !ok {
			return
		}

		effectiveJob := job
		if source, sourceErr := resolveEffectiveSourceJob(r.Context(), st, jobID); sourceErr == nil {
			effectiveJob.ID = source.ID
			effectiveJob.RunID = source.RunID
		} else {
			slog.Error("get job logs: resolve effective source failed", "job_id", jobID.String(), "err", sourceErr)
			writeHTTPError(w, http.StatusInternalServerError, "failed to resolve log source")
			return
		}

		hub := eventsService.Hub()
		if err := hub.EnsureJob(effectiveJob.ID); err != nil {
			slog.Error("ensure job stream failed", "job_id", jobID.String(), "err", err)
			writeHTTPError(w, http.StatusBadRequest, "invalid job id")
			return
		}

		allowedJobs := map[domaintypes.JobID]struct{}{effectiveJob.ID: {}}
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// For fresh connections, backfill historical logs then subscribe to job stream.
		if sinceID == 0 && bs != nil {
			if serveJobWithBackfill(w, r, st, bs, hub, effectiveJob, allowedJobs) {
				return
			}
		}

		// Non-zero sinceID or backfill setup failed: subscribe directly to job stream.
		if err := logstream.ServeJob(w, r, hub, effectiveJob.ID, sinceID); err != nil {
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
	requireBlobPersist("createJobLogsHandler", bp)
	requireEventsService("createJobLogsHandler", eventsService)
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		job, ok := getJobOrFail(w, r, st, jobID, "job logs ingest")
		if !ok {
			return
		}

		if rejectOversizedContentLength(w, r, ingestMaxBodySize) {
			return
		}

		var req struct {
			ChunkNo int32  `json:"chunk_no"`
			Data    []byte `json:"data"`
		}

		if err := decodeRequestJSON(w, r, &req, ingestMaxBodySize); err != nil {
			return
		}

		if len(req.Data) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "data is required and must not be empty")
			return
		}
		if len(req.Data) > ingestMaxDataSize {
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

		writeJSON(w, http.StatusCreated, map[string]any{"id": logRow.ID, "chunk_no": logRow.ChunkNo})
	}
}
