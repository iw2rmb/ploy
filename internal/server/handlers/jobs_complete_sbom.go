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

	latestGateJob, ok := latestSuccessfulGateJob(jobs)
	if !ok {
		return 0, nil
	}

	rows, err := bp.ExtractSBOMRowsForJob(ctx, runID, latestGateJob.ID, repoID)
	if err != nil {
		return 0, fmt.Errorf("extract sbom rows for latest successful gate job %s: %w", latestGateJob.ID, err)
	}

	for _, candidate := range jobs {
		if !isSBOMGateJobType(candidate.JobType) {
			continue
		}
		if err := st.DeleteSBOMRowsByJob(ctx, candidate.ID); err != nil {
			return 0, fmt.Errorf("delete sbom rows for gate job %s: %w", candidate.ID, err)
		}
	}

	for _, row := range rows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  row.JobID,
			RepoID: row.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return 0, fmt.Errorf("upsert sbom row for gate job %s: %w", latestGateJob.ID, err)
		}
	}
	return len(rows), nil
}

func latestSuccessfulGateJob(jobs []store.Job) (store.Job, bool) {
	var latest store.Job
	found := false
	for _, job := range jobs {
		if !isSBOMGateJobType(job.JobType) || job.Status != domaintypes.JobStatusSuccess {
			continue
		}
		if !found || job.ID.String() > latest.ID.String() {
			latest = job
			found = true
		}
	}
	return latest, found
}

func isSBOMGateJobType(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		return true
	default:
		return false
	}
}
