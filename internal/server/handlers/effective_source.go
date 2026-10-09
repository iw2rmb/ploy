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
	bundles, err := st.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: job.RunID,
		JobID: &job.ID,
	})
	if err != nil {
		return nil, err
	}
	current := bundles[:0]
	for _, bundle := range bundles {
		if bundle.Name != nil && (*bundle.Name == "sbom" || *bundle.Name == "cves") {
			// A failed-step resume reuses the job ID but resets started_at.
			if !job.StartedAt.Valid || !bundle.CreatedAt.Valid || bundle.CreatedAt.Time.Before(job.StartedAt.Time) {
				continue
			}
		}
		current = append(current, bundle)
	}
	return current, nil
}
