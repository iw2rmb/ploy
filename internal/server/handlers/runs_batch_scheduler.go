package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/batchscheduler"
)

// StartPendingReposResult is an alias to batchscheduler.StartPendingReposResult.
// This ensures the handlers package and batchscheduler package use the same type,
// allowing BatchRepoStarter to implement the batchscheduler.RepoStarter interface.
type StartPendingReposResult = batchscheduler.StartPendingReposResult

type batchRepoStarterStore interface {
	GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error)
	GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error)
	ListRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error)
	ListQueuedRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error)
	ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error)
	UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error
	ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error)
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
}

// BatchRepoStarter starts execution for pending repos in batch runs.
// It implements the batchscheduler.RepoStarter interface.
type BatchRepoStarter struct {
	store batchRepoStarterStore
}

// NewBatchRepoStarter creates a new BatchRepoStarter with the given store.
func NewBatchRepoStarter(st batchRepoStarterStore) *BatchRepoStarter {
	return &BatchRepoStarter{store: st}
}

// StartPendingRepos creates (or advances) repo-scoped job queues for queued run_repos rows.
// v1 removes per-repo execution runs; jobs are created directly for (run_id, repo_id, attempt).
func (s *BatchRepoStarter) StartPendingRepos(ctx context.Context, runID domaintypes.RunID) (StartPendingReposResult, error) {
	result := StartPendingReposResult{}
	runIDStr := runID.String()

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("get run: %w", err)
	}

	// Skip terminal runs — no more repos to start.
	if isTerminalRunStatus(run.Status) {
		return result, nil
	}

	spec, err := s.store.GetSpec(ctx, run.SpecID)
	if err != nil {
		return result, fmt.Errorf("get spec: %w", err)
	}

	allRepos, err := s.store.ListRunReposByRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("list run repos: %w", err)
	}
	for _, rr := range allRepos {
		if isTerminalRunRepoStatus(rr.Status) {
			result.AlreadyDone++
			continue
		}
		if rr.Status == store.RunRepoStatusQueued {
			result.Pending++
		}
	}

	queuedRepos, err := s.store.ListQueuedRunReposByRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("list queued repos: %w", err)
	}

	if len(queuedRepos) == 0 {
		return result, nil
	}

	for _, rr := range queuedRepos {
		jobs, err := s.store.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  rr.RepoID,
			Attempt: rr.Attempt,
		})
		if err != nil {
			slog.Error("start queued repos: list jobs failed", "run_id", runIDStr, "repo_id", rr.RepoID, "attempt", rr.Attempt, "err", err)
			continue
		}

		if len(jobs) == 0 {
			if err := createJobsFromSpec(ctx, s.store, runID, rr.RepoID, rr.RepoBaseRef, rr.Attempt, spec.Spec); err != nil {
				slog.Error("start queued repos: create jobs failed", "run_id", runIDStr, "repo_id", rr.RepoID, "attempt", rr.Attempt, "err", err)
				if updateErr := s.store.UpdateRunRepoError(ctx, store.UpdateRunRepoErrorParams{RunID: runID, RepoID: rr.RepoID, LastError: ptr(fmt.Sprintf("create jobs: %v", err))}); updateErr != nil {
					slog.Error("start queued repos: update repo error failed", "run_id", runIDStr, "repo_id", rr.RepoID, "err", updateErr)
				}
				continue
			}
			result.Started++
			continue
		}

		hasActive := false
		for _, j := range jobs {
			if j.Status == store.JobStatusQueued || j.Status == store.JobStatusRunning {
				hasActive = true
				break
			}
		}
		if hasActive {
			continue
		}

		if _, err := s.store.ScheduleNextJob(ctx, store.ScheduleNextJobParams{RunID: runID, RepoID: rr.RepoID, Attempt: rr.Attempt}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("start queued repos: schedule next job failed", "run_id", runIDStr, "repo_id", rr.RepoID, "attempt", rr.Attempt, "err", err)
			continue
		}
		result.Started++
	}

	return result, nil
}

func ptr[T any](v T) *T { return &v }
