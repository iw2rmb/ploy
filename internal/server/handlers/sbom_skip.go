package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveAndPersistSBOMStepSkip(
	ctx context.Context,
	st store.Store,
	job store.Job,
	mergedSpec []byte,
) (*contracts.SBOMStepSkipMetadata, error) {
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeSBOM {
		return nil, nil
	}

	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil {
		return nil, nil
	}

	cacheKey, err := computeJobCacheKey(
		domaintypes.JobTypeSBOM,
		job.Name,
		job.JobImage,
		job.RepoShaIn,
		"",
		mergedSpec,
	)
	if err != nil {
		return nil, fmt.Errorf("compute sbom cache key: %w", err)
	}
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return nil, nil
	}
	if err := pgStore.UpdateJobCacheKey(ctx, store.UpdateJobCacheKeyParams{
		ID:       job.ID,
		CacheKey: cacheKey,
	}); err != nil {
		return nil, fmt.Errorf("persist sbom cache key: %w", err)
	}

	var skip *contracts.SBOMStepSkipMetadata
	row, err := pgStore.ResolveReusableSBOMByCacheKey(ctx, store.ResolveReusableSBOMByCacheKeyParams{
		RepoID:   job.RepoID,
		CacheKey: cacheKey,
	})
	if err == nil {
		if row.RefArtifactID != "" && strings.TrimSpace(row.RefJobImage) != "" {
			skip = &contracts.SBOMStepSkipMetadata{
				Enabled:       true,
				RefArtifactID: strings.TrimSpace(row.RefArtifactID),
				RefJobImage:   strings.TrimSpace(row.RefJobImage),
			}
		}
	} else if !isNoRowsError(err) {
		return nil, fmt.Errorf("resolve reusable sbom: %w", err)
	}

	return skip, nil
}
