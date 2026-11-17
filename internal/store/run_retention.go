package store

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

type RunCleanupState struct {
	Run
	CleanupEligible bool
}

func runRetentionExpired(run Run, now time.Time) bool {
	if !run.FinishedAt.Valid {
		return false
	}
	var retention time.Duration
	switch run.Status {
	case types.RunStatusSuccess:
		retention = 6 * time.Hour
	case types.RunStatusFail, types.RunStatusCancelled:
		retention = 24 * time.Hour
	default:
		return false
	}
	return !now.Before(run.FinishedAt.Time.Add(retention))
}

// GetRunCleanupState serializes eligibility with restart and completion. Callers
// must recheck the returned identity immediately before deleting local data.
func (s *PgStore) GetRunCleanupState(ctx context.Context, id types.RunID) (RunCleanupState, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunCleanupState{}, err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	run, err := q.GetRunForUpdate(ctx, id)
	if err != nil {
		return RunCleanupState{}, err
	}
	// Transaction-start time can predate a resume that held the run lock.
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return RunCleanupState{}, err
	}
	state := RunCleanupState{Run: run}
	if runRetentionExpired(run, now) {
		jobs, err := q.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: id, Attempt: run.Attempt})
		if err != nil {
			return RunCleanupState{}, err
		}
		state.CleanupEligible = len(jobs) > 0
		for _, job := range jobs {
			switch job.Status {
			case types.JobStatusSuccess, types.JobStatusFail, types.JobStatusError, types.JobStatusCancelled:
			default:
				state.CleanupEligible = false
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RunCleanupState{}, err
	}
	return state, nil
}
