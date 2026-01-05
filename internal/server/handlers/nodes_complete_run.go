package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// maybeCompleteMultiStepRun is kept for handler compatibility, but v1 completion is repo-aggregate:
// - runs.status transitions to Finished only when all run_repos are terminal (Success/Fail/Cancelled).
// - success/failure is derived from repo outcomes (not stored on runs.status).
func maybeCompleteMultiStepRun(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run, runID domaintypes.RunID) error {
	if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
		return nil
	}

	counts, err := st.CountRunReposByStatus(ctx, runID.String())
	if err != nil {
		return fmt.Errorf("count run repos: %w", err)
	}

	var (
		total        int32
		terminal     int32
		anyFail      bool
		anyCancelled bool
	)
	for _, row := range counts {
		total += row.Count
		switch row.Status {
		case store.RunRepoStatusSuccess, store.RunRepoStatusFail, store.RunRepoStatusCancelled:
			terminal += row.Count
		}
		if row.Status == store.RunRepoStatusFail && row.Count > 0 {
			anyFail = true
		}
		if row.Status == store.RunRepoStatusCancelled && row.Count > 0 {
			anyCancelled = true
		}
	}

	if total == 0 || terminal < total {
		return nil
	}

	if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: runID.String(), Status: store.RunStatusFinished}); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	if eventsService != nil {
		repoURL := ""
		if repos, err := st.ListRunReposByRun(ctx, runID.String()); err == nil && len(repos) > 0 {
			if mr, err := st.GetModRepo(ctx, repos[0].RepoID); err == nil {
				repoURL = mr.RepoUrl
			}
		}

		runState := modsapi.RunStateSucceeded
		if anyFail {
			runState = modsapi.RunStateFailed
		} else if anyCancelled {
			runState = modsapi.RunStateCancelled
		}

		summary := modsapi.RunSummary{
			RunID:      runID,
			State:      runState,
			Repository: repoURL,
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}
		if err := eventsService.PublishRun(ctx, runID, summary); err != nil {
			slog.Error("complete run: publish run event failed", "run_id", runID, "err", err)
		}
		if err := eventsService.Hub().PublishStatus(ctx, runID.String(), logstream.Status{Status: "done"}); err != nil {
			slog.Error("complete run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("run completed", "run_id", runID, "status", store.RunStatusFinished)
	return nil
}
