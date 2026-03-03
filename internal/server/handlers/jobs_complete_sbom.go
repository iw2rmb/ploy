package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func maybePersistGateSuccessSBOMRows(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	job store.Job,
	status store.JobStatus,
) (int, error) {
	if bp == nil || status != store.JobStatusSuccess {
		return 0, nil
	}
	if !isSBOMGateJobType(domaintypes.JobType(strings.TrimSpace(job.JobType))) {
		return 0, nil
	}

	rows, err := bp.ExtractSBOMRowsForJob(ctx, job.RunID, job.ID, job.RepoID)
	if err != nil {
		return 0, fmt.Errorf("extract sbom rows for gate job %s: %w", job.ID, err)
	}
	if err := st.DeleteSBOMRowsByJob(ctx, job.ID); err != nil {
		return 0, fmt.Errorf("delete sbom rows for gate job %s: %w", job.ID, err)
	}

	for _, row := range rows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  row.JobID,
			RepoID: row.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return 0, fmt.Errorf("upsert sbom row for gate job %s: %w", job.ID, err)
		}
	}
	return len(rows), nil
}

func isSBOMGateJobType(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		return true
	default:
		return false
	}
}
