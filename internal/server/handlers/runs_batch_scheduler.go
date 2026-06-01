package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/batchscheduler"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// BatchRepoStarter starts execution for pending repos in batch runs.
// It implements the batchscheduler.RepoStarter interface.
type BatchRepoStarter struct {
	store store.Store
	bs    blobstore.Store
}

// NewBatchRepoStarter creates a new BatchRepoStarter with the given store.
func NewBatchRepoStarter(st store.Store, bs blobstore.Store) *BatchRepoStarter {
	return &BatchRepoStarter{store: st, bs: bs}
}

// StartPendingRepos creates (or advances) job queues for queued runs in a wave.
func (s *BatchRepoStarter) StartPendingRepos(ctx context.Context, waveID domaintypes.WaveID) (batchscheduler.StartPendingReposResult, error) {
	result := batchscheduler.StartPendingReposResult{}

	wave, err := s.store.GetWave(ctx, waveID)
	if err != nil {
		return result, fmt.Errorf("get wave: %w", err)
	}

	if lifecycle.IsTerminalWaveStatus(wave.Status) {
		return result, nil
	}

	spec, err := s.store.GetSpec(ctx, wave.SpecID)
	if err != nil {
		return result, fmt.Errorf("get spec: %w", err)
	}

	allRuns, err := s.store.ListRunsByWave(ctx, waveID)
	if err != nil {
		return result, fmt.Errorf("list runs by wave: %w", err)
	}
	for _, run := range allRuns {
		if lifecycle.IsTerminalRunStatus(run.Status) {
			result.AlreadyDone++
			continue
		}
		if run.Status == domaintypes.RunStatusQueued {
			result.Pending++
		}
	}

	queuedRuns, err := s.store.ListQueuedRunsByWave(ctx, waveID)
	if err != nil {
		return result, fmt.Errorf("list queued runs: %w", err)
	}

	if len(queuedRuns) == 0 {
		return result, nil
	}

	for _, run := range queuedRuns {
		jobs, err := s.store.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
			RunID:   run.ID,
			RepoID:  run.RepoID,
			Attempt: run.Attempt,
		})
		if err != nil {
			slog.Error("start queued runs: list jobs failed", "run_id", run.ID, "repo_id", run.RepoID, "attempt", run.Attempt, "err", err)
			continue
		}

		if len(jobs) == 0 {
			if err := createJobsFromSpec(ctx, s.store, run.ID, run.RepoID, run.RepoBaseRef, run.Attempt, run.RepoSha0, spec.Spec, s.bs); err != nil {
				slog.Error("start queued runs: create jobs failed", "run_id", run.ID, "repo_id", run.RepoID, "attempt", run.Attempt, "err", err)
				if updateErr := s.store.UpdateRunError(ctx, store.UpdateRunErrorParams{ID: run.ID, LastError: ptr(fmt.Sprintf("create jobs: %v", err))}); updateErr != nil {
					slog.Error("start queued runs: update run error failed", "run_id", run.ID, "repo_id", run.RepoID, "err", updateErr)
				}
				continue
			}
			if err := s.store.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: run.ID, Status: domaintypes.RunStatusRunning}); err != nil {
				slog.Error("start queued runs: mark running failed", "run_id", run.ID, "err", err)
				continue
			}
			result.Started++
			continue
		}

		hasActive := false
		for _, j := range jobs {
			if j.Status == domaintypes.JobStatusQueued || j.Status == domaintypes.JobStatusRunning {
				hasActive = true
				break
			}
		}
		if hasActive {
			continue
		}

		if _, err := s.store.ScheduleNextJob(ctx, store.ScheduleNextJobParams{RunID: run.ID, RepoID: run.RepoID, Attempt: run.Attempt}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("start queued runs: schedule next job failed", "run_id", run.ID, "repo_id", run.RepoID, "attempt", run.Attempt, "err", err)
			continue
		}
		if err := s.store.UpdateRunStatus(ctx, store.UpdateRunStatusParams{ID: run.ID, Status: domaintypes.RunStatusRunning}); err != nil {
			slog.Error("start queued runs: mark running failed", "run_id", run.ID, "err", err)
			continue
		}
		result.Started++
	}

	return result, nil
}

func ptr[T any](v T) *T { return &v }
