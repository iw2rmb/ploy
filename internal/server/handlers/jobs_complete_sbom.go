package handlers

import (
	"context"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func maybePersistLatestSuccessfulCycleSBOMRows(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	attempt int32,
) (int, error) {
	if bp == nil {
		return 0, nil
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return 0, fmt.Errorf("list jobs for sbom persistence: %w", err)
	}

	latestSBOMJob, ok := latestSuccessfulSBOMJob(jobs)
	if !ok {
		return 0, nil
	}

	rows, err := bp.ExtractSBOMRowsForJob(ctx, runID, latestSBOMJob.ID, repoID)
	if err != nil {
		return 0, fmt.Errorf("extract sbom rows for latest successful sbom job %s: %w", latestSBOMJob.ID, err)
	}

	for _, candidate := range jobs {
		if !isSBOMJobType(candidate.JobType) {
			continue
		}
		if err := st.DeleteSBOMRowsByJob(ctx, candidate.ID); err != nil {
			return 0, fmt.Errorf("delete sbom rows for sbom job %s: %w", candidate.ID, err)
		}
	}

	for _, row := range rows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  row.JobID,
			RepoID: row.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return 0, fmt.Errorf("upsert sbom row for sbom job %s: %w", latestSBOMJob.ID, err)
		}
	}
	return len(rows), nil
}

func latestSuccessfulSBOMJob(jobs []store.Job) (store.Job, bool) {
	var latest store.Job
	found := false
	for _, job := range jobs {
		if !isSBOMJobType(job.JobType) || job.Status != domaintypes.JobStatusSuccess {
			continue
		}
		if !found || sbomJobIsMoreRecent(job, latest) {
			latest = job
			found = true
		}
	}
	return latest, found
}

func sbomJobIsMoreRecent(candidate, current store.Job) bool {
	// Prefer explicit completion recency for successful jobs.
	if candidate.FinishedAt.Valid != current.FinishedAt.Valid {
		return candidate.FinishedAt.Valid
	}
	if candidate.FinishedAt.Valid && !candidate.FinishedAt.Time.Equal(current.FinishedAt.Time) {
		return candidate.FinishedAt.Time.After(current.FinishedAt.Time)
	}

	// Fall back to start time when finished_at is unavailable.
	if candidate.StartedAt.Valid != current.StartedAt.Valid {
		return candidate.StartedAt.Valid
	}
	if candidate.StartedAt.Valid && !candidate.StartedAt.Time.Equal(current.StartedAt.Time) {
		return candidate.StartedAt.Time.After(current.StartedAt.Time)
	}

	// Deterministic tie-break when timestamps are equal.
	return candidate.ID.String() > current.ID.String()
}

func isSBOMJobType(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypeSBOM
}
