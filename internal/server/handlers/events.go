package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
	"github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// backfillKey uniquely identifies a backfilled log line so that dedup only
// suppresses true overlaps (same timestamp + stream + content), not legitimate
// live lines that happen to share the same text.
type backfillKey struct {
	Timestamp string
	Stream    string
	Line      string
}

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

// getRunLogsHandler returns an HTTP handler that streams run lifecycle events over SSE.
// Supports Last-Event-ID header for resuming streams from a specific event.
// GET /v1/runs/{run_id}/logs — SSE for run lifecycle (run, stage, done only).
//
// Container log frames are not emitted on this stream; they are served via the
// job-scoped log endpoint (GET /v1/jobs/{job_id}/logs).
//
// For terminal runs, the handler writes a "done" sentinel immediately.
// For active runs, the handler subscribes to the hub for live lifecycle events.
func getRunLogsHandler(st store.Store, _ blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run logs")
		if !ok {
			return
		}

		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))
		hub := eventsService.Hub()

		if err := hub.Ensure(runID); err != nil {
			slog.Error("ensure stream failed", "run_id", runID.String(), "err", err)
			writeHTTPError(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// For fresh connections to terminal runs, write done immediately.
		if sinceID == 0 && lifecycle.IsTerminalRunStatus(run.Status) {
			if serveRunTerminalDone(w, run) {
				return
			}
		}

		// Subscribe to hub for live lifecycle events (run, stage, done).
		// Reject log/retention frames that may exist in run stream history.
		if err := logstream.ServeFiltered(w, r, hub, runID, sinceID, buildRunLifecycleFilter()); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run logs", "run_id", runID.String(), "err", err)
			}
		}
	}
}

// serveRunTerminalDone writes SSE headers and a done sentinel for terminal runs.
// Returns true if the response was fully handled.
func serveRunTerminalDone(w http.ResponseWriter, run store.Run) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return true
	}
	flusher.Flush()
	doneData, _ := json.Marshal(map[string]string{"status": string(run.Status)})
	_ = logstream.WriteEventFrame(w, logstream.Event{Type: domaintypes.SSEEventDone, Data: doneData})
	flusher.Flush()
	return true
}

// serveJobWithBackfill writes SSE headers, backfills historical logs for a specific
// job from the database, and for active jobs transitions to live job-stream
// subscription. Returns true if the response was fully handled.
func serveJobWithBackfill(w http.ResponseWriter, r *http.Request, st store.Store, bs blobstore.Store, hub *logstream.Hub, job store.Job, allowedJobs map[domaintypes.JobID]struct{}) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	if _, err := io.WriteString(w, ":ok\n\n"); err != nil {
		return true
	}
	flusher.Flush()

	// Capture the hub high-water mark BEFORE backfill so we can identify
	// gap events (published during backfill) afterward.
	preCursor := hubHighWater(hub.SnapshotJob(job.ID))

	// Backfill historical logs filtered to this job.
	backfilledLines, err := backfillRunLogs(r.Context(), w, flusher, st, bs, job.RunID, allowedJobs)
	if err != nil {
		slog.Error("backfill job logs failed", "job_id", job.ID.String(), "err", err)
	}

	// Replay all gap events that arrived during backfill (ID > preCursor),
	// deduplicating against lines already emitted by backfill. A log
	// published to the hub during backfill may also have been persisted to
	// the DB fast enough to appear in ListLogsByRun results; without dedup
	// the client would see such a line twice.
	postSnapshot := hub.SnapshotJob(job.ID)
	postCursor := preCursor
	for _, evt := range postSnapshot {
		if evt.ID <= preCursor {
			continue
		}
		postCursor = evt.ID
		if evt.Type == domaintypes.SSEEventLog && backfilledLines != nil {
			var rec logstream.LogRecord
			if json.Unmarshal(evt.Data, &rec) == nil {
				key := backfillKey{Timestamp: rec.Timestamp, Stream: rec.Stream, Line: rec.Line}
				if _, dup := backfilledLines[key]; dup {
					delete(backfilledLines, key)
					continue
				}
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

	// If job is terminal, replay any retention events from the snapshot
	// that were published before the handler ran (id <= preCursor) and
	// therefore skipped by the gap replay above, then write done.
	if isTerminalJobStatus(job.Status) {
		for _, evt := range postSnapshot {
			if evt.Type == domaintypes.SSEEventRetention && evt.ID <= preCursor {
				if err := logstream.WriteEventFrame(w, evt); err != nil {
					return true
				}
				flusher.Flush()
			}
		}
		doneData, _ := json.Marshal(map[string]string{"status": string(job.Status)})
		_ = logstream.WriteEventFrame(w, logstream.Event{Type: domaintypes.SSEEventDone, Data: doneData})
		flusher.Flush()
		return true
	}

	sub, err := hub.SubscribeJob(r.Context(), job.ID, postCursor)
	if err != nil {
		slog.Error("subscribe after backfill failed", "job_id", job.ID.String(), "err", err)
		return true
	}
	defer sub.Cancel()

	for {
		select {
		case <-r.Context().Done():
			return true
		case evt, ok := <-sub.Events:
			if !ok {
				return true
			}
			if evt.Type == domaintypes.SSEEventLog && backfilledLines != nil {
				var rec logstream.LogRecord
				if json.Unmarshal(evt.Data, &rec) == nil {
					key := backfillKey{Timestamp: rec.Timestamp, Stream: rec.Stream, Line: rec.Line}
					if _, dup := backfilledLines[key]; dup {
						delete(backfilledLines, key)
						continue
					}
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

// hubHighWater returns the ID of the last event in the snapshot, or 0 if empty.
func hubHighWater(snapshot []logstream.Event) domaintypes.EventID {
	if len(snapshot) == 0 {
		return 0
	}
	return snapshot[len(snapshot)-1].ID
}

// isTerminalJobStatus reports whether the job status is terminal.
func isTerminalJobStatus(status domaintypes.JobStatus) bool {
	switch status {
	case domaintypes.JobStatusSuccess, domaintypes.JobStatusFail,
		domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
		return true
	default:
		return false
	}
}

// backfillRunLogs fetches historical log chunks from the database and object store,
// decompresses them, and writes them as SSE frames. If allowedJobs is non-nil, only
// logs belonging to those jobs are included.
func backfillRunLogs(ctx context.Context, w io.Writer, flusher http.Flusher, st store.Store, bs blobstore.Store, runID domaintypes.RunID, allowedJobs map[domaintypes.JobID]struct{}) (map[backfillKey]struct{}, error) {
	logs, err := st.ListLogsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	emitted := make(map[backfillKey]struct{})
	for _, lg := range logs {
		// Filter by allowed jobs if specified.
		if allowedJobs != nil {
			if lg.JobID == nil {
				continue
			}
			if _, ok := allowedJobs[*lg.JobID]; !ok {
				continue
			}
		}

		if lg.ObjectKey == nil || *lg.ObjectKey == "" {
			continue
		}

		if err := backfillOneChunk(ctx, w, bs, lg, emitted); err != nil {
			slog.Warn("backfill chunk failed", "run_id", runID.String(), "log_id", lg.ID, "err", err)
			continue
		}
		flusher.Flush()
	}
	return emitted, nil
}

// backfillOneChunk fetches a single gzipped log chunk from object store,
// decompresses it, and writes each line as an SSE log frame.
func backfillOneChunk(ctx context.Context, w io.Writer, bs blobstore.Store, lg store.Log, emitted map[backfillKey]struct{}) error {
	data, err := blobstore.ReadAll(ctx, bs, *lg.ObjectKey)
	if err != nil {
		return err
	}
	records, err := logchunk.DecodeGzip(data)
	if err != nil {
		return err
	}

	ts := timestampToString(lg.CreatedAt)

	var jobID domaintypes.JobID
	if lg.JobID != nil {
		jobID = *lg.JobID
	}
	for _, frame := range records {
		emitted[backfillKey{Timestamp: ts, Stream: frame.Stream, Line: frame.Line}] = struct{}{}
		rec := logstream.LogRecord{
			Timestamp: ts,
			Stream:    frame.Stream,
			Line:      frame.Line,
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
	return nil
}

// timestampToString converts a pgtype.Timestamptz to RFC3339 string.
func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}

// --- Merged from events_repo.go ---

// getRunRepoLogsHandler returns an HTTP handler that streams run lifecycle events
// over SSE, filtered to stages belonging to a specific repo execution.
// GET /v1/runs/{run_id}/repos/{repo_id}/logs
//
// Container log frames are not emitted on this stream (logs moved to job-scoped
// streams). Only run, stage, and done events are returned.
func getRunRepoLogsHandler(st store.Store, _ blobstore.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseRequiredPathID[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				writeHTTPError(w, http.StatusNotFound, "repo not found")
			default:
				slog.Error("get run repo logs: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
				writeHTTPError(w, http.StatusInternalServerError, "failed to get repo")
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
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs")
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
			writeHTTPError(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// For fresh connections to terminal runs, write done immediately.
		if sinceID == 0 && lifecycle.IsTerminalRunStatus(run.Status) {
			if serveRunTerminalDone(w, run) {
				return
			}
		}

		// Stream lifecycle-only frames, filtering run events to repo's stages.
		filter := buildRepoLifecycleFilter(allowedJobs)

		if err := logstream.ServeFiltered(w, r, hub, runID, sinceID, filter); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run repo logs", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
		}
	}
}

// buildRunLifecycleFilter returns a filter that passes only run lifecycle events
// (run, stage, done) and rejects log and retention frames.
func buildRunLifecycleFilter() func(logstream.Event) (logstream.Event, bool) {
	return func(evt logstream.Event) (logstream.Event, bool) {
		switch evt.Type {
		case domaintypes.SSEEventRun, domaintypes.SSEEventStage, domaintypes.SSEEventDone:
			return evt, true
		default:
			return evt, false
		}
	}
}

// buildRepoLifecycleFilter returns a filter that passes lifecycle events
// (run, stage, done) and filters run event stages to the allowed jobs set.
// Log and retention events are rejected (they no longer appear on the run stream).
func buildRepoLifecycleFilter(allowedJobs map[domaintypes.JobID]struct{}) func(logstream.Event) (logstream.Event, bool) {
	return func(evt logstream.Event) (logstream.Event, bool) {
		switch evt.Type {
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
		case domaintypes.SSEEventStage, domaintypes.SSEEventDone:
			return evt, true
		default:
			// Reject log/retention if any stale events appear.
			return evt, false
		}
	}
}
