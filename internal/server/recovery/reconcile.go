package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// MaybeUpdateRunStatus derives and persists runs.status from job outcomes.
func MaybeUpdateRunStatus(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	attempt int32,
) (bool, error) {
	jobs, err := st.ListJobsByRunAttempt(ctx, store.ListJobsByRunAttemptParams{
		RunID:   runID,
		Attempt: attempt,
	})
	if err != nil {
		return false, fmt.Errorf("list jobs by run attempt: %w", err)
	}

	eval, err := EvaluateRunAttemptTerminalStatus(jobs)
	if err != nil {
		return false, err
	}
	if !eval.ShouldUpdate {
		return false, nil
	}

	if err := st.UpdateRunStatus(ctx, store.UpdateRunStatusParams{
		ID:     runID,
		Status: eval.Status,
	}); err != nil {
		return false, fmt.Errorf("update run status: %w", err)
	}

	slog.Info("run completed",
		"run_id", runID,
		"attempt", attempt,
		"status", eval.Status,
	)

	return true, nil
}

// MaybeCompleteRunIfAllReposTerminal completes the owning wave when all child runs are terminal.
func MaybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *events.Service, run store.Run) (bool, error) {
	runID := run.ID
	wave, err := st.GetWave(ctx, run.WaveID)
	if err != nil {
		return false, fmt.Errorf("get wave: %w", err)
	}
	if lifecycle.IsTerminalWaveStatus(wave.Status) {
		return false, nil
	}

	counts, err := st.CountRunsByWaveStatus(ctx, run.WaveID)
	if err != nil {
		return false, fmt.Errorf("count wave runs: %w", err)
	}

	eval := lifecycle.EvaluateWaveCompletionFromRunCounts(counts)
	if !eval.ShouldFinish {
		return false, nil
	}

	currentWave, err := st.GetWave(ctx, run.WaveID)
	if err != nil {
		return false, fmt.Errorf("get wave for completion check: %w", err)
	}
	if lifecycle.IsTerminalWaveStatus(currentWave.Status) {
		return false, nil
	}

	if err := st.UpdateWaveStatus(ctx, store.UpdateWaveStatusParams{ID: run.WaveID, Status: domaintypes.WaveStatusFinished}); err != nil {
		return false, fmt.Errorf("update wave status: %w", err)
	}

	if eventsService != nil {
		repoURL, _ := repoURLForID(ctx, st, run.RepoID)

		summary := migsapi.RunSummary{
			RunID:      runID,
			State:      eval.RunState,
			Repository: repoURL,
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[domaintypes.JobID]migsapi.StageStatus),
		}
		if err := eventsService.PublishRun(ctx, runID, summary); err != nil {
			slog.Error("complete run: publish run event failed", "run_id", runID, "err", err)
		}
		if err := eventsService.Hub().PublishStatus(ctx, runID, logstream.Status{Status: "done"}); err != nil {
			slog.Error("complete run: publish done status failed", "run_id", runID, "err", err)
		}
	}

	slog.Info("wave completed", "wave_id", run.WaveID, "status", domaintypes.WaveStatusFinished)
	return true, nil
}

func repoURLForID(ctx context.Context, st store.Store, repoID domaintypes.RepoID) (string, error) {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return "", err
	}
	return repo.Url, nil
}

func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
