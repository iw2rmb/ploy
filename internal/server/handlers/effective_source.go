package handlers

import (
	"context"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// resolveEffectiveSourceJob resolves the source job for content reads.
// Non-mirrored jobs resolve to themselves.
func resolveEffectiveSourceJob(ctx context.Context, st store.Store, jobID domaintypes.JobID) (store.Job, error) {
	job, err := st.GetJob(ctx, jobID)
	if err != nil {
		return store.Job{}, err
	}

	if len(job.Meta) == 0 {
		return job, nil
	}
	meta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || meta.CacheMirror == nil {
		return job, nil
	}
	sourceID := meta.CacheMirror.SourceJobID
	if sourceID.IsZero() {
		return store.Job{}, fmt.Errorf("cache_mirror.source_job_id is required")
	}
	if sourceID == job.ID {
		return store.Job{}, fmt.Errorf("cache_mirror.source_job_id must not equal job id %s", job.ID)
	}

	source, err := st.GetJob(ctx, sourceID)
	if err != nil {
		return store.Job{}, fmt.Errorf("get effective source job %s: %w", sourceID, err)
	}
	if len(source.Meta) == 0 {
		return source, nil
	}
	sourceMeta, err := contracts.UnmarshalJobMeta(source.Meta)
	if err == nil && sourceMeta.CacheMirror != nil {
		return store.Job{}, fmt.Errorf("cache_mirror source job %s must not be mirrored", sourceID)
	}
	return source, nil
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
