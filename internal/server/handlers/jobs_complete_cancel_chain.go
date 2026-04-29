package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// cancelRemainingJobsAfterFailure cancels non-terminal jobs reachable from the failed job's successor chain.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	failedJob store.Job,
) error {
	now := time.Now().UTC()

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}

	nextID := failedJob.NextID
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		nextID = refreshed.NextID
	}

	seen := map[domaintypes.JobID]struct{}{}
	for nextID != nil {
		if _, dup := seen[*nextID]; dup {
			break
		}
		seen[*nextID] = struct{}{}

		job, ok := jobsByID[*nextID]
		if !ok {
			break
		}
		nextID = job.NextID

		switch job.Status {
		case domaintypes.JobStatusSuccess, domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
			continue
		}

		startedAt := job.StartedAt
		var durationMs int64
		if job.StartedAt.Valid {
			durationMs = now.Sub(job.StartedAt.Time).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}

		finishedAt := pgtype.Timestamptz{Time: now, Valid: true}
		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     domaintypes.JobStatusCancelled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", job.ID, err)
		}

		slog.Info("canceled linked job after failure",
			"run_id", failedJob.RunID,
			"failed_job_id", failedJob.ID,
			"job_id", job.ID,
		)
	}

	return nil
}
