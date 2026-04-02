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
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeMig {
		return nil
	}

	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil {
		return nil
	}

	stepRow, err := pgStore.GetStepByJob(ctx, job.ID.String())
	if isNoRowsError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load steps cache row: %w", err)
	}
	if stepRow.RefJobID == nil || strings.TrimSpace(*stepRow.RefJobID) == "" {
		return nil
	}

	targetJobID := job.ID
	if _, err := pgStore.GetLatestDiffByJob(ctx, &targetJobID); err == nil {
		return nil
	} else if !isNoRowsError(err) {
		return fmt.Errorf("check existing target diff: %w", err)
	}

	refJobID := domaintypes.JobID(strings.TrimSpace(*stepRow.RefJobID))
	if refJobID.IsZero() {
		return nil
	}

	if err := bp.CloneLatestDiffByJob(ctx, refJobID.String(), job.RunID.String(), job.ID.String()); err != nil {
		return err
	}
	return nil
}
