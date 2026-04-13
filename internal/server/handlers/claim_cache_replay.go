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

	cacheKey, err := computeJobCacheKey(domaintypes.JobType(job.JobType), job.Meta, job.JobImage, job.RepoShaIn, runtimeInputHash, payload.Spec)
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
	if !isReplayStatusEligible(domaintypes.JobType(job.JobType), replayStatus, candidate.ExitCode) {
		return false, nil
	}
	repoSHAOut := strings.TrimSpace(candidate.RepoShaOut)
	if replayStatus == domaintypes.JobStatusSuccess && repoSHAOut == "" && isNonChangingJob(domaintypes.JobType(job.JobType)) {
		repoSHAOut = normalizeRepoSHA(job.RepoShaIn)
	}
	if replayStatus == domaintypes.JobStatusSuccess && job.NextID != nil && repoSHAOut == "" {
		return false, nil
	}
	if replayStatus == domaintypes.JobStatusSuccess {
		ok, diffErr := hasReplayableSourceDiff(ctx, s.store, sourceJob)
		if diffErr != nil {
			return false, fmt.Errorf("check replayable source diff for job %s: %w", sourceJob.ID, diffErr)
		}
		if !ok {
			return false, nil
		}
	}

	var sbomTargetSnapshot []store.Sbom
	sbomRowsCopied := false
	if replayStatus == domaintypes.JobStatusSuccess && domaintypes.JobType(job.JobType) == domaintypes.JobTypeSBOM {
		snapshot, copyErr := copySBOMRowsFromSourceToTarget(ctx, s.store, sourceJob, job)
		if copyErr != nil {
			return false, fmt.Errorf("copy mirrored sbom rows source=%s target=%s: %w", sourceJob.ID, job.ID, copyErr)
		}
		sbomTargetSnapshot = snapshot
		sbomRowsCopied = true
	}

	mirroredMeta, ok := replayMirroredJobMeta(sourceJob.ID, candidate.Meta)
	if !ok {
		// Replay must remain transparent: if mirror metadata cannot be attached,
		// treat this candidate as ineligible and let regular execution proceed.
		if sbomRowsCopied {
			if restoreErr := restoreSBOMRowsForTarget(ctx, s.store, job.ID, sbomTargetSnapshot); restoreErr != nil {
				return false, fmt.Errorf("rollback copied sbom rows after metadata rejection for target=%s: %w", job.ID, restoreErr)
			}
		}
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
		if sbomRowsCopied {
			if restoreErr := restoreSBOMRowsForTarget(ctx, s.store, job.ID, sbomTargetSnapshot); restoreErr != nil {
				return false, fmt.Errorf("replay completion for cached hit source=%s target=%s failed: %w (rollback sbom rows failed: %v)", sourceJob.ID, job.ID, err, restoreErr)
			}
		}
		return false, fmt.Errorf("replay completion for cached hit source=%s target=%s: %w", sourceJob.ID, job.ID, err)
	}
	return true, nil
}

func isReplayStatusEligible(jobType domaintypes.JobType, status domaintypes.JobStatus, exitCode *int32) bool {
	if status == domaintypes.JobStatusSuccess {
		return true
	}
	if status != domaintypes.JobStatusFail {
		return false
	}
	if jobType == domaintypes.JobTypeHook || jobType == domaintypes.JobTypeHeal {
		return false
	}
	return exitCode != nil && *exitCode == 1
}

func (s *ClaimService) resolveRuntimeInputHash(ctx context.Context, job store.Job, payload claimResponsePayload) (string, bool, error) {
	components := map[string]string{}
	jobType := domaintypes.JobType(job.JobType)

	if payload.DetectedStack != nil {
		_, hash, err := canonicalizeAndHashJSON(payload.DetectedStack)
		if err != nil {
			return "", false, fmt.Errorf("hash detected stack input: %w", err)
		}
		components["detected_stack"] = hash
	}

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

func hasReplayableSourceDiff(ctx context.Context, st store.Store, sourceJob store.Job) (bool, error) {
	if !canChangeWorkspace(domaintypes.JobType(sourceJob.JobType)) {
		return true, nil
	}
	if normalizeRepoSHA(sourceJob.RepoShaOut) == normalizeRepoSHA(sourceJob.RepoShaIn) {
		return false, nil
	}
	sourceJobID := sourceJob.ID
	if _, err := st.GetLatestDiffByJob(ctx, &sourceJobID); err != nil {
		if isNoRowsError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func copySBOMRowsFromSourceToTarget(
	ctx context.Context,
	st store.Store,
	sourceJob store.Job,
	targetJob store.Job,
) ([]store.Sbom, error) {
	existingTargetRows, err := st.ListSBOMRowsByJob(ctx, targetJob.ID)
	if err != nil {
		return nil, fmt.Errorf("list target sbom rows: %w", err)
	}
	sourceRows, err := st.ListSBOMRowsByJob(ctx, sourceJob.ID)
	if err != nil {
		return nil, fmt.Errorf("list source sbom rows: %w", err)
	}
	if err := st.DeleteSBOMRowsByJob(ctx, targetJob.ID); err != nil {
		return nil, fmt.Errorf("delete target sbom rows: %w", err)
	}
	for _, row := range sourceRows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  targetJob.ID,
			RepoID: targetJob.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return nil, fmt.Errorf("insert mirrored sbom row %q@%q: %w", row.Lib, row.Ver, err)
		}
	}
	return existingTargetRows, nil
}

func restoreSBOMRowsForTarget(
	ctx context.Context,
	st store.Store,
	targetJobID domaintypes.JobID,
	rows []store.Sbom,
) error {
	if err := st.DeleteSBOMRowsByJob(ctx, targetJobID); err != nil {
		return fmt.Errorf("delete target sbom rows before restore: %w", err)
	}
	for _, row := range rows {
		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  targetJobID,
			RepoID: row.RepoID,
			Lib:    row.Lib,
			Ver:    row.Ver,
		}); err != nil {
			return fmt.Errorf("restore sbom row %q@%q: %w", row.Lib, row.Ver, err)
		}
	}
	return nil
}
