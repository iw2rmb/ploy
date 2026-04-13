package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func maybeCloneSkippedStepDiffBeforeCompletion(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	job store.Job,
) error {
	if bp == nil {
		return nil
	}
	jobType := domaintypes.JobType(job.JobType)
	if !canChangeWorkspace(jobType) {
		return nil
	}

	sourceJob, err := resolveEffectiveSourceJob(ctx, st, job.ID)
	if err != nil {
		return fmt.Errorf("resolve effective source job: %w", err)
	}
	if sourceJob.ID == job.ID {
		return nil
	}

	targetJobID := job.ID
	if _, err := st.GetLatestDiffByJob(ctx, &targetJobID); err == nil {
		return nil
	} else if !isNoRowsError(err) {
		return fmt.Errorf("check existing target diff: %w", err)
	}
	sourceJobID := sourceJob.ID
	if _, err := st.GetLatestDiffByJob(ctx, &sourceJobID); err != nil {
		if isNoRowsError(err) {
			return fmt.Errorf("source mirrored %s job %s has no diff to clone", strings.TrimSpace(string(sourceJob.JobType)), sourceJob.ID)
		}
		return fmt.Errorf("check source diff: %w", err)
	}

	if err := bp.CloneLatestDiffByJob(ctx, sourceJob.ID.String(), job.RunID.String(), job.ID.String()); err != nil {
		return err
	}
	return nil
}
