package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var cacheReplayDigestHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *ClaimService) tryReplayCachedOutcome(
	ctx context.Context,
	nodeID domaintypes.NodeID,
	job store.Job,
	payload claimResponsePayload,
) (bool, error) {
	pgStore, ok := s.store.(*store.PgStore)
	if !ok || pgStore == nil {
		return false, nil
	}

	runtimeInputHash, eligible, err := s.resolveRuntimeInputHash(ctx, job, payload)
	if err != nil {
		return false, fmt.Errorf("resolve runtime input hash: %w", err)
	}
	if !eligible {
		return false, nil
	}

	cacheKey, err := computeJobCacheKey(domaintypes.JobType(job.JobType), job.Name, job.JobImage, job.RepoShaIn, runtimeInputHash, payload.Spec)
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

	mirroredMeta, ok := replayMirroredJobMeta(sourceJob.ID, candidate.Meta)
	if !ok {
		// Replay must remain transparent: if mirror metadata cannot be attached,
		// treat this candidate as ineligible and let regular execution proceed.
		return false, nil
	}
	stats := JobStatsPayload{JobMeta: mirroredMeta}
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

func (s *ClaimService) resolveRuntimeInputHash(ctx context.Context, job store.Job, payload claimResponsePayload) (string, bool, error) {
	components := map[string]string{}
	jobType := domaintypes.JobType(job.JobType)

	switch jobType {
	case domaintypes.JobTypeHook:
		if payload.HookRuntime == nil {
			return "", false, nil
		}
		_, hash, err := canonicalizeAndHashJSON(payload.HookRuntime)
		if err != nil {
			return "", false, fmt.Errorf("hash hook runtime input: %w", err)
		}
		components["hook_runtime"] = hash
	case domaintypes.JobTypeHeal, domaintypes.JobTypeReGate:
		if payload.RecoveryContext == nil {
			return "", false, nil
		}
		_, hash, err := canonicalizeAndHashJSON(payload.RecoveryContext)
		if err != nil {
			return "", false, fmt.Errorf("hash recovery runtime input: %w", err)
		}
		components["recovery_context"] = hash
	}

	sbomDigest, required, available, err := s.resolveUpstreamSBOMInputHash(ctx, job)
	if err != nil {
		return "", false, err
	}
	if required && !available {
		return "", false, nil
	}
	if available {
		components["upstream_sbom_digest"] = sbomDigest
	}

	if len(components) == 0 {
		return "", true, nil
	}

	keys := make([]string, 0, len(components))
	for k := range components {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := make([]byte, 0, 256)
	for _, k := range keys {
		buf = append(buf, k...)
		buf = append(buf, '=')
		buf = append(buf, components[k]...)
		buf = append(buf, '\n')
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), true, nil
}

func (s *ClaimService) resolveUpstreamSBOMInputHash(ctx context.Context, job store.Job) (string, bool, bool, error) {
	jobs, err := s.store.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return "", false, false, fmt.Errorf("list run repo jobs: %w", err)
	}

	var predecessor *store.Job
	for idx := range jobs {
		nextID := jobs[idx].NextID
		if nextID == nil || *nextID != job.ID {
			continue
		}
		if predecessor != nil {
			return "", false, false, fmt.Errorf("multiple predecessor jobs reference %s", job.ID)
		}
		predecessor = &jobs[idx]
	}
	if predecessor == nil {
		return "", false, false, nil
	}
	if domaintypes.JobType(predecessor.JobType) != domaintypes.JobTypeSBOM {
		return "", false, false, nil
	}
	if domaintypes.JobStatus(predecessor.Status) != domaintypes.JobStatusSuccess {
		return "", false, false, nil
	}

	source, sourceErr := resolveEffectiveSourceJob(ctx, s.store, predecessor.ID)
	if sourceErr != nil {
		// Fail-open: treat unresolved mirrored content as replay ineligible.
		return "", true, false, nil
	}

	bundles, err := s.store.ListArtifactBundlesByRunAndJob(ctx, store.ListArtifactBundlesByRunAndJobParams{
		RunID: source.RunID,
		JobID: &source.ID,
	})
	if err != nil {
		return "", true, false, fmt.Errorf("list predecessor sbom artifacts: %w", err)
	}
	for _, bundle := range bundles {
		if bundle.Name != nil && strings.TrimSpace(*bundle.Name) != "" && strings.TrimSpace(*bundle.Name) != "mig-out" {
			continue
		}
		digest := ""
		if bundle.Digest != nil {
			digest = strings.ToLower(strings.TrimSpace(*bundle.Digest))
		}
		digest = strings.TrimPrefix(digest, "sha256:")
		if !cacheReplayDigestHexPattern.MatchString(digest) {
			return "", true, false, nil
		}
		return digest, true, true, nil
	}
	return "", true, false, nil
}

func replayMirroredJobMeta(sourceJobID domaintypes.JobID, candidateRaw []byte) ([]byte, bool) {
	meta, err := contracts.UnmarshalJobMeta(candidateRaw)
	if err != nil {
		return nil, false
	}
	if meta.CacheMirror != nil {
		// Disallow mirror->mirror selection.
		return nil, false
	}
	meta.CacheMirror = &contracts.CacheMirrorMetadata{SourceJobID: sourceJobID}
	out, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return nil, false
	}
	return out, true
}
