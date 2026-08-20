package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/store"
)

func listArtifactBundlesByJob(
	ctx context.Context,
	st store.Store,
	job store.Job,
) ([]store.ArtifactBundle, error) {
	return st.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: job.RunID,
		JobID: &job.ID,
	})
}
