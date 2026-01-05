package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// cancelRunHandler cancels a Mods run and transitions it to a terminal state.
// POST /v1/mods/{id}/cancel — Optional JSON body { reason?: string }
// Responses:
//   - 202 Accepted on state transition
//   - 200 OK if already terminal (idempotent)
//   - 404 Not Found if run does not exist
//   - 400 Bad Request for invalid id
func cancelRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Optional body: { reason?: string }
		var req struct {
			Reason *string `json:"reason"`
		}
		// Empty body is allowed; decode only if body has data
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Load current run
		run, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("cancel run: lookup failed", "run_id", runIDStr, "err", err)
			return
		}

		// If already terminal, idempotent 200 OK
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Transition to Cancelled; finished_at is set by the DB on terminal transition.
		now := time.Now().UTC()
		err = st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{
			ID:     runIDStr,
			Status: store.RunStatusCancelled,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to cancel run: %v", err), http.StatusInternalServerError)
			slog.Error("cancel run: update run failed", "run_id", runIDStr, "err", err)
			return
		}

		// Cancel all queued/running repos for this run.
		if repos, err := st.ListRunReposByRun(r.Context(), runIDStr); err == nil {
			for _, rr := range repos {
				if rr.Status != store.RunRepoStatusQueued && rr.Status != store.RunRepoStatusRunning {
					continue
				}
				_ = st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{
					RunID:  rr.RunID,
					RepoID: rr.RepoID,
					Status: store.RunRepoStatusCancelled,
				})
			}
		}

		// Best-effort job updates to canceled — only for created|pending|running jobs
		if jobs, err := st.ListJobsByRun(r.Context(), runIDStr); err == nil && len(jobs) > 0 {
			for _, job := range jobs {
				if job.Status != store.JobStatusCreated && job.Status != store.JobStatusQueued && job.Status != store.JobStatusRunning {
					continue
				}
				// Compute duration if started
				dur := int64(0)
				if job.StartedAt.Valid {
					d := now.Sub(job.StartedAt.Time).Milliseconds()
					if d > 0 {
						dur = d
					}
				}
				_ = st.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
					ID:         job.ID,
					Status:     store.JobStatusCancelled,
					StartedAt:  job.StartedAt,
					FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
					DurationMs: dur,
				})
			}
		}

		// Publish terminal run event + done status for SSE clients.
		if eventsService != nil {
			repoURL := ""
			if repos, err := st.ListRunReposByRun(r.Context(), runIDStr); err == nil && len(repos) > 0 {
				if mr, err := st.GetModRepo(r.Context(), repos[0].RepoID); err == nil {
					repoURL = mr.RepoUrl
				}
			}

			// Construct RunSummary with RunID for SSE event publishing.
			runSummary := modsapi.RunSummary{
				RunID:      domaintypes.RunID(runIDStr),
				State:      modsapi.RunStateCancelled,
				Repository: repoURL,
				CreatedAt:  timeOrZero(run.CreatedAt),
				UpdatedAt:  now,
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if req.Reason != nil && strings.TrimSpace(*req.Reason) != "" {
				if runSummary.Metadata == nil {
					runSummary.Metadata = map[string]string{}
				}
				runSummary.Metadata["reason"] = strings.TrimSpace(*req.Reason)
			}
			if err := eventsService.PublishRun(r.Context(), domaintypes.RunID(runIDStr), runSummary); err != nil {
				slog.Error("cancel run: publish run event failed", "run_id", runIDStr, "err", err)
			}
			// Signal done on the stream.
			if err := eventsService.Hub().PublishStatus(r.Context(), runIDStr, logstream.Status{Status: "done"}); err != nil {
				slog.Error("cancel run: publish done status failed", "run_id", runIDStr, "err", err)
			}
		}

		w.WriteHeader(http.StatusAccepted)
		slog.Info("run canceled", "run_id", runIDStr, "had_reason", req.Reason != nil)
	}
}
