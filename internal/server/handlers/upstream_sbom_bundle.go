package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

var upstreamSBOMDigestHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type upstreamSBOMBundle struct {
	Digest     string
	ArtifactID string
}

func resolveUpstreamSBOMBundleForJob(
	ctx context.Context,
	st store.Store,
	job store.Job,
) (upstreamSBOMBundle, bool, bool, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return upstreamSBOMBundle{}, false, false, fmt.Errorf("list run repo jobs: %w", err)
	}

	var predecessor *store.Job
	for idx := range jobs {
		nextID := jobs[idx].NextID
		if nextID == nil || *nextID != job.ID {
			continue
		}
		if predecessor != nil {
			return upstreamSBOMBundle{}, false, false, fmt.Errorf("multiple predecessor jobs reference %s", job.ID)
		}
		predecessor = &jobs[idx]
	}
	if predecessor == nil {
		return upstreamSBOMBundle{}, false, false, nil
	}
	if domaintypes.JobType(predecessor.JobType) != domaintypes.JobTypeSBOM {
		return upstreamSBOMBundle{}, false, false, nil
	}
	if predecessor.Status != domaintypes.JobStatusSuccess {
		return upstreamSBOMBundle{}, true, false, nil
	}

	bundles, listErr := st.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: predecessor.RunID,
		JobID: &predecessor.ID,
	})
	if listErr != nil {
		return upstreamSBOMBundle{}, true, false, fmt.Errorf("list predecessor sbom artifacts: %w", listErr)
	}
	for _, bundle := range bundles {
		name := strings.TrimSpace(pointerString(bundle.Name))
		if name != "sbom-file" {
			continue
		}
		digest := strings.ToLower(strings.TrimSpace(pointerString(bundle.Digest)))
		if !upstreamSBOMDigestHexPattern.MatchString(digest) {
			return upstreamSBOMBundle{}, true, false, nil
		}
		return upstreamSBOMBundle{
			Digest:     digest,
			ArtifactID: bundle.ID.String(),
		}, true, true, nil
	}
	return upstreamSBOMBundle{}, true, false, nil
}

func pointerString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
