package handlers

import (
	"context"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// resolveEffectiveSourceJob resolves the source job for content reads.
// Content is read from the job itself.
func resolveEffectiveSourceJob(ctx context.Context, st store.Store, jobID domaintypes.JobID) (store.Job, error) {
	return st.GetJob(ctx, jobID)
}

func listArtifactBundlesByEffectiveJob(
	ctx context.Context,
	st store.Store,
	job store.Job,
) ([]store.ArtifactBundle, error) {
	source, err := resolveEffectiveSourceJob(ctx, st, job.ID)
	if err != nil {
		return nil, err
	}
	return st.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: source.RunID,
		JobID: &source.ID,
	})
}

func listSBOMRowsByEffectiveJob(
	ctx context.Context,
	st store.Store,
	job store.Job,
) ([]store.Sbom, error) {
	source, err := resolveEffectiveSourceJob(ctx, st, job.ID)
	if err != nil {
		return nil, err
	}
	return st.ListSBOMRowsByJob(ctx, source.ID)
}
