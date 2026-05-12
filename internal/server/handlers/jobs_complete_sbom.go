package handlers

import (
	"context"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func maybePersistSBOMRowsForJob(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	jobID domaintypes.JobID,
) (int, error) {
	if bp == nil {
		return 0, nil
	}
	rows, err := bp.ExtractSBOMRowsForJob(ctx, runID, jobID, repoID)
	if err != nil {
		return 0, fmt.Errorf("extract sbom rows for job %s: %w", jobID, err)
	}
	if err := st.DeleteSBOMRowsByJob(ctx, jobID); err != nil {
		return 0, fmt.Errorf("delete sbom rows for job %s: %w", jobID, err)
	}
	for _, row := range rows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  row.JobID,
			RepoID: row.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return 0, fmt.Errorf("upsert sbom row for job %s: %w", jobID, err)
		}
	}
	return len(rows), nil
}
