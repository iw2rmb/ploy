package handlers

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// parseLastEventID parses the Last-Event-ID header to support SSE resumption.
// Returns 0 if the header is absent, invalid, or negative.
// Uses types.EventID for type-safe cursor handling.
func parseLastEventID(header string) domaintypes.EventID {
	if header == "" {
		return 0
	}
	var eid domaintypes.EventID
	if err := eid.UnmarshalText([]byte(header)); err != nil {
		return 0
	}
	if !eid.Valid() {
		return 0
	}
	return eid
}

// getRunLogsHandler returns an HTTP handler that streams run logs and events over SSE.
// Supports Last-Event-ID header for resuming streams from a specific event.
// GET /v1/runs/{id}/logs — Native SSE for run logs/events.
//
// When sinceID is 0 (fresh connection), the handler backfills historical logs
// from the database and object store before subscribing to the live hub.
// For terminal runs (Finished/Cancelled), the backfill is followed by a "done"
// event and the stream ends. For active runs, the handler transitions to live
// streaming from the hub after backfill completes.
func getRunLogsHandler(st store.Store, bs blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run logs")
		if !ok {
			return
		}

		// Parse Last-Event-ID header for resumption support.
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// Get the hub from the events service.
		hub := eventsService.Hub()

		// Ensure the stream exists (creates if not present).
		if err := hub.Ensure(runID); err != nil {
			slog.Error("ensure stream failed", "run_id", runID.String(), "err", err)
			httpErr(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// For fresh connections, backfill historical logs from DB + object store.
		if sinceID == 0 && bs != nil {
			if serveWithBackfill(w, r, st, bs, hub, run, runID, nil) {
				return
			}
			// If backfill setup failed (e.g., no flusher), fall through to pure hub streaming.
		}

		// Non-zero sinceID or backfill not available: use existing pure-hub path.
		if err := logstream.Serve(w, r, hub, runID, sinceID); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run logs", "run_id", runID.String(), "err", err)
			}
		}
	}
}

// serveWithBackfill writes SSE headers, backfills historical logs, and for active
// runs transitions to live hub streaming. Returns true if the response was fully
// handled (caller should return), false if the caller should fall through.
//
// If allowedJobs is non-nil, backfill and live events are filtered to those jobs only.
func serveWithBackfill(w http.ResponseWriter, r *http.Request, st store.Store, bs blobstore.Store, hub *logstream.Hub, run store.Run, runID domaintypes.RunID, allowedJobs map[domaintypes.JobID]struct{}) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}

	// Write SSE headers.
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return true
	}
	flusher.Flush()

	// Backfill historical logs from DB + Garage.
	if err := backfillRunLogs(r.Context(), w, flusher, st, bs, runID, allowedJobs); err != nil {
		slog.Error("backfill logs failed", "run_id", runID.String(), "err", err)
	}

	// If run is terminal, write done and return.
	if isTerminalRunStatus(run.Status) {
		doneData, _ := json.Marshal(map[string]string{"status": string(run.Status)})
		_ = logstream.WriteEventFrame(w, logstream.Event{Type: domaintypes.SSEEventDone, Data: doneData})
		flusher.Flush()
		return true
	}

	// Run is still active — subscribe to hub from current high-water mark
	// to avoid duplicating events already sent during backfill.
	snapshot := hub.Snapshot(runID)
	var hubSinceID domaintypes.EventID
	if len(snapshot) > 0 {
		hubSinceID = snapshot[len(snapshot)-1].ID
	}

	sub, err := hub.Subscribe(r.Context(), runID, hubSinceID)
	if err != nil {
		slog.Error("subscribe after backfill failed", "run_id", runID.String(), "err", err)
		return true
	}
	defer sub.Cancel()

	// Build filter for repo-scoped streaming (nil for unfiltered).
	var filter func(logstream.Event) (logstream.Event, bool)
	if allowedJobs != nil {
		filter = buildRepoLogFilter(allowedJobs)
	}

	for {
		select {
		case <-r.Context().Done():
			return true
		case evt, ok := <-sub.Events:
			if !ok {
				return true
			}
			if filter != nil {
				var keep bool
				evt, keep = filter(evt)
				if !keep {
					continue
				}
			}
			if err := logstream.WriteEventFrame(w, evt); err != nil {
				return true
			}
			flusher.Flush()
			if evt.Type == domaintypes.SSEEventDone {
				return true
			}
		}
	}
}

// backfillRunLogs fetches historical log chunks from the database and object store,
// decompresses them, and writes them as SSE frames. If allowedJobs is non-nil, only
// logs belonging to those jobs are included.
func backfillRunLogs(ctx context.Context, w io.Writer, flusher http.Flusher, st store.Store, bs blobstore.Store, runID domaintypes.RunID, allowedJobs map[domaintypes.JobID]struct{}) error {
	logs, err := st.ListLogsByRun(ctx, runID)
	if err != nil {
		return err
	}

	for _, lg := range logs {
		// Filter by allowed jobs if specified.
		if allowedJobs != nil && lg.JobID != nil {
			if _, ok := allowedJobs[*lg.JobID]; !ok {
				continue
			}
		}

		if lg.ObjectKey == nil || *lg.ObjectKey == "" {
			continue
		}

		if err := backfillOneChunk(ctx, w, bs, lg); err != nil {
			slog.Warn("backfill chunk failed", "run_id", runID.String(), "log_id", lg.ID, "err", err)
			continue
		}
		flusher.Flush()
	}
	return nil
}

// backfillOneChunk fetches a single gzipped log chunk from object store,
// decompresses it, and writes each line as an SSE log frame.
func backfillOneChunk(ctx context.Context, w io.Writer, bs blobstore.Store, lg store.Log) error {
	rc, _, err := bs.Get(ctx, *lg.ObjectKey)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	zr, err := gzip.NewReader(rc)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	ts := timestampToString(lg.CreatedAt)

	var jobID domaintypes.JobID
	if lg.JobID != nil {
		jobID = *lg.JobID
	}

	scanner := bufio.NewScanner(zr)
	const maxLine = 256 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLine)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		rec := logstream.LogRecord{
			Timestamp: ts,
			Stream:    "stdout",
			Line:      line,
			JobID:     jobID,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		// Write without an event ID — these are historical, not resumable.
		if err := logstream.WriteEventFrame(w, logstream.Event{
			Type: domaintypes.SSEEventLog,
			Data: data,
		}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// timestampToString converts a pgtype.Timestamptz to RFC3339 string.
func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}

// --- Merged from events_repo.go ---

// getRunRepoLogsHandler returns an HTTP handler that streams run logs/events over SSE,
// filtered to jobs belonging to a specific repo execution within the run.
// GET /v1/runs/{run_id}/repos/{repo_id}/logs
func getRunRepoLogsHandler(st store.Store, bs blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseParam[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				httpErr(w, http.StatusNotFound, "repo not found")
			default:
				slog.Error("get run repo logs: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
				httpErr(w, http.StatusInternalServerError, "failed to get repo")
			}
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run repo logs")
		if !ok {
			return
		}

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: rr.Attempt,
		})
		if err != nil {
			slog.Error("get run repo logs: list jobs failed", "run_id", runID.String(), "repo_id", repoID.String(), "attempt", rr.Attempt, "err", err)
			httpErr(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}

		allowedJobs := make(map[domaintypes.JobID]struct{}, len(jobs))
		for _, job := range jobs {
			allowedJobs[job.ID] = struct{}{}
		}

		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))
		hub := eventsService.Hub()
		if err := hub.Ensure(runID); err != nil {
			slog.Error("ensure stream failed", "run_id", runID.String(), "err", err)
			httpErr(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// For fresh connections, backfill historical logs filtered by allowed jobs.
		if sinceID == 0 && bs != nil {
			if serveWithBackfill(w, r, st, bs, hub, run, runID, allowedJobs) {
				return
			}
		}

		filter := buildRepoLogFilter(allowedJobs)

		if err := logstream.ServeFiltered(w, r, hub, runID, sinceID, filter); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run repo logs", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
		}
	}
}

// buildRepoLogFilter returns a filter function that passes only log events
// belonging to the allowed jobs set.
func buildRepoLogFilter(allowedJobs map[domaintypes.JobID]struct{}) func(logstream.Event) (logstream.Event, bool) {
	return func(evt logstream.Event) (logstream.Event, bool) {
		switch evt.Type {
		case domaintypes.SSEEventLog:
			var payload struct {
				JobID *string `json:"job_id,omitempty"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return evt, true
			}
			if payload.JobID == nil || strings.TrimSpace(*payload.JobID) == "" {
				return evt, false
			}
			var jobID domaintypes.JobID
			if err := jobID.UnmarshalText([]byte(*payload.JobID)); err != nil {
				return evt, false
			}
			_, ok := allowedJobs[jobID]
			return evt, ok
		case domaintypes.SSEEventRun:
			var summary api.RunSummary
			if err := json.Unmarshal(evt.Data, &summary); err != nil {
				return evt, true
			}
			if len(summary.Stages) == 0 {
				return evt, true
			}
			for jobID := range summary.Stages {
				if _, ok := allowedJobs[jobID]; !ok {
					delete(summary.Stages, jobID)
				}
			}
			data, err := json.Marshal(summary)
			if err != nil {
				return evt, true
			}
			evt.Data = data
			return evt, true
		default:
			return evt, true
		}
	}
}
