package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

type missingCachedReplayArtifactError struct {
	SourceJobID domaintypes.JobID
	ObjectKey   string
	Err         error
}

func (e *missingCachedReplayArtifactError) Error() string {
	if e == nil {
		return ""
	}
	src := strings.TrimSpace(e.SourceJobID.String())
	key := strings.TrimSpace(e.ObjectKey)
	switch {
	case src == "" && key == "":
		return fmt.Sprintf("cached replay source blob is missing: %v", e.Err)
	case key == "":
		return fmt.Sprintf("cached replay source blob is missing for source job %s: %v", src, e.Err)
	case src == "":
		return fmt.Sprintf("cached replay source blob is missing (%s): %v", key, e.Err)
	default:
		return fmt.Sprintf("cached replay source blob is missing for source job %s (%s): %v", src, key, e.Err)
	}
}

func (e *missingCachedReplayArtifactError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func recoverableCacheReplayMissingArtifact(err error) *missingCachedReplayArtifactError {
	var target *missingCachedReplayArtifactError
	if errors.As(err, &target) {
		return target
	}
	return nil
}

func (s *ClaimService) tryReplayCachedOutcome(
	ctx context.Context,
	nodeID domaintypes.NodeID,
	job store.Job,
	mergedSpec []byte,
) (bool, error) {
	pgStore, ok := s.store.(*store.PgStore)
	if !ok || pgStore == nil {
		return false, nil
	}

	cacheKey, err := computeJobCacheKey(domaintypes.JobType(job.JobType), job.Name, job.JobImage, job.RepoShaIn, mergedSpec)
	if err != nil {
		return false, fmt.Errorf("compute cache key: %w", err)
	}
	if strings.TrimSpace(cacheKey) == "" {
		return false, nil
	}
	if err := pgStore.UpdateJobCacheKey(ctx, store.UpdateJobCacheKeyParams{
		ID:       job.ID,
		CacheKey: cacheKey,
	}); err != nil {
		return false, fmt.Errorf("persist cache key: %w", err)
	}

	candidate, err := pgStore.ResolveReusableJobByCacheKey(ctx, store.ResolveReusableJobByCacheKeyParams{
		RepoID:   job.RepoID,
		JobType:  job.JobType,
		CacheKey: cacheKey,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("resolve reusable job by cache key: %w", err)
	}
	if candidate.ID == job.ID {
		return false, nil
	}

	sourceJob, err := s.store.GetJob(ctx, candidate.ID)
	if err != nil {
		return false, fmt.Errorf("load source cached job %s: %w", candidate.ID, err)
	}
	if err := s.cloneReplayOutputs(ctx, sourceJob, job); err != nil {
		return false, fmt.Errorf("clone cached outputs from %s: %w", sourceJob.ID, err)
	}

	replayStatus := domaintypes.JobStatus(candidate.Status)
	if replayStatus != domaintypes.JobStatusSuccess && replayStatus != domaintypes.JobStatusFail {
		return false, nil
	}
	repoSHAOut := strings.TrimSpace(candidate.RepoShaOut)
	if replayStatus == domaintypes.JobStatusSuccess && repoSHAOut == "" && isNonChangingJob(domaintypes.JobType(job.JobType)) {
		repoSHAOut = normalizeRepoSHA(job.RepoShaIn)
	}
	if replayStatus == domaintypes.JobStatusSuccess && job.NextID != nil && repoSHAOut == "" {
		return false, nil
	}

	stats := JobStatsPayload{}
	meta := strings.TrimSpace(string(candidate.Meta))
	if meta != "" && meta != "{}" && meta != "null" {
		stats.JobMeta = append([]byte(nil), candidate.Meta...)
	}
	completeSvc := NewCompleteJobService(s.store, nil, blobpersist.New(s.store, s.blobStore), nil)
	_, err = completeSvc.Complete(ctx, CompleteJobInput{
		JobID:        job.ID,
		NodeID:       nodeID,
		Status:       replayStatus,
		ExitCode:     candidate.ExitCode,
		StatsPayload: stats,
		RepoSHAOut:   repoSHAOut,
	})
	if err != nil {
		return false, fmt.Errorf("replay completion for cached hit source=%s target=%s: %w", sourceJob.ID, job.ID, err)
	}
	return true, nil
}

func (s *ClaimService) cloneReplayOutputs(ctx context.Context, source, target store.Job) error {
	if s.blobStore == nil {
		return nil
	}
	bp := blobpersist.New(s.store, s.blobStore)
	if bp == nil {
		return nil
	}

	if err := bp.CloneLatestDiffByJob(ctx, source.ID.String(), target.RunID.String(), target.ID.String()); err != nil {
		if errors.Is(err, blobstore.ErrNotFound) {
			return &missingCachedReplayArtifactError{
				SourceJobID: source.ID,
				Err:         err,
			}
		}
		return err
	}

	targetBundles, err := s.store.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: target.RunID,
		JobID: &target.ID,
	})
	if err != nil {
		return fmt.Errorf("list target artifact bundles: %w", err)
	}
	if len(targetBundles) > 0 {
		return nil
	}

	sourceBundles, err := s.store.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: source.RunID,
		JobID: &source.ID,
	})
	if err != nil {
		return fmt.Errorf("list source artifact bundles: %w", err)
	}
	for _, bundle := range sourceBundles {
		if bundle.ObjectKey == nil || strings.TrimSpace(*bundle.ObjectKey) == "" {
			continue
		}
		payload, readErr := blobstore.ReadAll(ctx, s.blobStore, *bundle.ObjectKey)
		if readErr != nil {
			if errors.Is(readErr, blobstore.ErrNotFound) {
				return &missingCachedReplayArtifactError{
					SourceJobID: source.ID,
					ObjectKey:   strings.TrimSpace(*bundle.ObjectKey),
					Err:         readErr,
				}
			}
			return fmt.Errorf("read source artifact blob %s: %w", *bundle.ObjectKey, readErr)
		}
		_, createErr := bp.CreateArtifactBundle(ctx, store.CreateArtifactBundleParams{
			RunID:  target.RunID,
			JobID:  &target.ID,
			Name:   bundle.Name,
			Cid:    bundle.Cid,
			Digest: bundle.Digest,
		}, payload)
		if createErr != nil {
			return fmt.Errorf("clone source artifact %x: %w", bundle.ID.Bytes, createErr)
		}
	}
	return nil
}
