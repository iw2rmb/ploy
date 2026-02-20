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
	"github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

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
func getRunRepoLogsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := domaintypes.ParseModRepoIDParam(r, "repo_id")
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
