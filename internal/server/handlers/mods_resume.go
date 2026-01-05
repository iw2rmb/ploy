package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// resumeRunHandler restarts work for a terminal v1 run by creating a new attempt for failed repos.
func resumeRunHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		run, err := st.GetRun(ctx, runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("resume run: lookup failed", "run_id", runIDStr, "err", err)
			return
		}

		if run.Status == store.RunStatusStarted {
			w.WriteHeader(http.StatusOK)
			return
		}
		if run.Status != store.RunStatusFinished && run.Status != store.RunStatusCancelled {
			http.Error(w, fmt.Sprintf("run state=%s is not resumable", run.Status), http.StatusConflict)
			return
		}

		spec, err := st.GetSpec(ctx, run.SpecID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load spec: %v", err), http.StatusInternalServerError)
			slog.Error("resume run: get spec failed", "run_id", runIDStr, "spec_id", run.SpecID, "err", err)
			return
		}

		repos, err := st.ListRunReposByRun(ctx, runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("resume run: list run repos failed", "run_id", runIDStr, "err", err)
			return
		}

		var toRestart []store.RunRepo
		for _, rr := range repos {
			switch run.Status {
			case store.RunStatusCancelled:
				if rr.Status != store.RunRepoStatusSuccess {
					toRestart = append(toRestart, rr)
				}
			case store.RunStatusFinished:
				if rr.Status == store.RunRepoStatusFail {
					toRestart = append(toRestart, rr)
				}
			}
		}

		if len(toRestart) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: runIDStr, Status: store.RunStatusStarted}); err != nil {
			http.Error(w, fmt.Sprintf("failed to resume run: %v", err), http.StatusInternalServerError)
			slog.Error("resume run: update run status failed", "run_id", runIDStr, "err", err)
			return
		}

		for _, rr := range toRestart {
			if err := st.IncrementRunRepoAttempt(ctx, store.IncrementRunRepoAttemptParams{RunID: runIDStr, RepoID: rr.RepoID}); err != nil {
				http.Error(w, fmt.Sprintf("failed to restart repo: %v", err), http.StatusInternalServerError)
				slog.Error("resume run: increment attempt failed", "run_id", runIDStr, "repo_id", rr.RepoID, "err", err)
				return
			}

			updatedRR, err := st.GetRunRepo(ctx, store.GetRunRepoParams{RunID: runIDStr, RepoID: rr.RepoID})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to load repo: %v", err), http.StatusInternalServerError)
				slog.Error("resume run: get run repo failed", "run_id", runIDStr, "repo_id", rr.RepoID, "err", err)
				return
			}
			if err := createJobsFromSpec(ctx, st, runIDStr, updatedRR.RepoID, updatedRR.RepoBaseRef, updatedRR.Attempt, spec.Spec); err != nil {
				http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
				slog.Error("resume run: create jobs failed", "run_id", runIDStr, "repo_id", updatedRR.RepoID, "attempt", updatedRR.Attempt, "err", err)
				return
			}
		}

		if err := st.UpdateRunResume(ctx, runIDStr); err != nil {
			slog.Error("resume run: update resume stats failed", "run_id", runIDStr, "err", err)
		}

		if eventsService != nil {
			repoURL := ""
			repoBase := ""
			repoTarget := ""
			if len(repos) > 0 {
				repoBase = repos[0].RepoBaseRef
				repoTarget = repos[0].RepoTargetRef
				if mr, err := st.GetModRepo(ctx, repos[0].RepoID); err == nil {
					repoURL = mr.RepoUrl
				}
			}
			runSummary := modsapi.RunSummary{
				RunID:      domaintypes.RunID(runIDStr),
				State:      modsapi.RunStateRunning,
				Repository: repoURL,
				Metadata:   map[string]string{"repo_base_ref": repoBase, "repo_target_ref": repoTarget},
				CreatedAt:  timeOrZero(run.CreatedAt),
				UpdatedAt:  time.Now().UTC(),
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishRun(ctx, domaintypes.RunID(runIDStr), runSummary); err != nil {
				slog.Error("resume run: publish run event failed", "run_id", runIDStr, "err", err)
			}
		}

		w.WriteHeader(http.StatusAccepted)
		slog.Info("run resumed", "run_id", runIDStr, "repos_restarted", len(toRestart))
	}
}
