package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// MaybeUpdateRunRepoStatus derives and persists run_repos.status from job outcomes.
func MaybeUpdateRunRepoStatus(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	attempt int32,
) (bool, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return false, fmt.Errorf("list jobs by repo attempt: %w", err)
	}

	eval, err := EvaluateRepoAttemptTerminalStatus(jobs)
	if err != nil {
		return false, err
	}
	if !eval.ShouldUpdate {
		return false, nil
	}

	if err := st.UpdateRunRepoStatus(ctx, store.UpdateRunRepoStatusParams{
		RunID:  runID,
		RepoID: repoID,
		Status: eval.Status,
	}); err != nil {
		return false, fmt.Errorf("update run repo status: %w", err)
	}

	slog.Info("run repo completed",
		"run_id", runID,
		"repo_id", repoID,
		"attempt", attempt,
		"status", eval.Status,
	)

	return true, nil
}

// MaybeCompleteRunIfAllReposTerminal transitions runs.status to Finished only when
// all run_repos are terminal and publishes run/done SSE events.
func MaybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *server.EventsService, run store.Run) (bool, error) {
	runID := run.ID
	if run.Status == domaintypes.RunStatusFinished || run.Status == domaintypes.RunStatusCancelled {
		return false, nil
	}

	counts, err := st.CountRunReposByStatus(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("count run repos: %w", err)
	}

	eval := lifecycle.EvaluateRunCompletionFromRepoCounts(counts)
	if !eval.ShouldFinish {
		return false, nil
	}

	currentRun, err := st.GetRun(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("get run for completion check: %w", err)
	}
	if currentRun.Status == domaintypes.RunStatusFinished || currentRun.Status == domaintypes.RunStatusCancelled {
		return false, nil
	}

	if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: runID, Status: domaintypes.RunStatusFinished}); err != nil {
		return false, fmt.Errorf("update run status: %w", err)
	}

	if eventsService != nil {
		repoURL := ""
		if repos, err := st.ListRunReposWithURLByRun(ctx, runID); err == nil && len(repos) > 0 {
			repoURL = repos[0].RepoUrl
		}

		summary := modsapi.RunSummary{
			RunID:      runID,
			State:      eval.RunState,
			Repository: repoURL,
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[domaintypes.JobID]modsapi.StageStatus),
		}
		if err := eventsService.PublishRun(ctx, runID, summary); err != nil {
			slog.Error("complete run: publish run event failed", "run_id", runID, "err", err)
		}
		if err := eventsService.Hub().PublishStatus(ctx, runID, logstream.Status{Status: "done"}); err != nil {
			slog.Error("complete run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("run completed", "run_id", runID, "status", domaintypes.RunStatusFinished)
	return true, nil
}

func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
