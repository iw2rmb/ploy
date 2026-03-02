package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

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
// Run IDs are now KSUID-backed strings (27 characters). We perform a cheap
// length check to reject obviously invalid IDs before hitting the store; the
// database layer enforces existence.
func getRunLogsHandler(st store.Store, eventsService *server.EventsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify run exists in the database.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				httpErr(w, http.StatusNotFound, "run not found")
			default:
				slog.Error("get run logs: database error", "run_id", runID.String(), "err", err)
				httpErr(w, http.StatusInternalServerError, "failed to get run")
			}
			return
		}

		// Parse Last-Event-ID header for resumption support.
		sinceID := parseLastEventID(r.Header.Get("Last-Event-ID"))

		// Get the hub from the events service.
		hub := eventsService.Hub()

		// Ensure the stream exists (creates if not present).
		// Validation happens inside Ensure; errors are logged but stream proceeds.
		if err := hub.Ensure(runID); err != nil {
			slog.Error("ensure stream failed", "run_id", runID.String(), "err", err)
			httpErr(w, http.StatusBadRequest, "invalid run id")
			return
		}

		// Delegate to logstream.Serve for SSE streaming.
		if err := logstream.Serve(w, r, hub, runID, sinceID); err != nil {
			// Only log non-cancellation errors (client disconnect is normal).
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run logs", "run_id", runID.String(), "err", err)
			}
		}
	}
}

// --- Merged from events_repo.go ---

type optionalJobID domaintypes.JobID

func (v *optionalJobID) UnmarshalJSON(b []byte) error {
	if v == nil {
		return errors.New("optionalJobID: UnmarshalJSON on nil pointer")
	}
	if string(b) == "null" {
		*v = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		*v = ""
		return nil
	}
	var id domaintypes.JobID
	if err := id.UnmarshalText([]byte(s)); err != nil {
		return err
	}
	*v = optionalJobID(id)
	return nil
}

// getRunRepoLogsHandler returns an HTTP handler that streams run logs/events over SSE,
// filtered to jobs belonging to a specific repo execution within the run.
// GET /v1/runs/{run_id}/repos/{repo_id}/logs
func getRunRepoLogsHandler(st store.Store, eventsService *server.EventsService) http.HandlerFunc {
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

		filter := func(evt logstream.Event) (logstream.Event, bool) {
			switch evt.Type {
			case domaintypes.SSEEventLog:
				// Filter log events by job_id.
				var payload struct {
					JobID optionalJobID `json:"job_id,omitempty"`
				}
				if err := json.Unmarshal(evt.Data, &payload); err != nil {
					return evt, true
				}
				jobID := domaintypes.JobID(payload.JobID)
				if jobID.IsZero() {
					return evt, false
				}
				_, ok := allowedJobs[jobID]
				return evt, ok
			case domaintypes.SSEEventRun:
				// Filter stages map to this repo's jobs (payload schema stays RunSummary).
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

		if err := logstream.ServeFiltered(w, r, hub, runID, sinceID, filter); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("stream run repo logs", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
		}
	}
}
