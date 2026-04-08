package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

type completeRunCache struct {
	run store.Run
	ok  bool
}

type completeJobState struct {
	input         CompleteJobInput
	job           store.Job
	jobType       domaintypes.JobType
	serviceType   completeJobServiceType
	serviceTypeOK bool
	persistedMeta []byte
	runCache      completeRunCache
}

func (s *CompleteJobService) Complete(ctx context.Context, input CompleteJobInput) (CompleteJobResult, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CompleteJobResult{}, completeNotFound("job not found")
		}
		return CompleteJobResult{}, completeInternal("failed to get job", err)
	}

	if job.NodeID == nil || *job.NodeID != input.NodeID {
		return CompleteJobResult{}, completeForbidden("job not assigned to this node")
	}
	if job.Status != domaintypes.JobStatusRunning {
		return CompleteJobResult{}, completeConflict("job status is %s, expected Running", job.Status)
	}
	jobType := domaintypes.JobType(job.JobType)
	serviceType, serviceTypeOK := routeCompleteJobServiceType(jobType)
	if !serviceTypeOK {
		slog.Error("complete job: invalid job_type in job record; treating as non-gate for post-completion routing",
			"job_id", input.JobID,
			"job_type", job.JobType,
		)
	}

	if input.Status == domaintypes.JobStatusSuccess && job.NextID != nil {
		if !sha40Pattern.MatchString(job.RepoShaIn) {
			return CompleteJobResult{}, completeConflict("job repo_sha_in must match ^[0-9a-f]{40}$ for chain progression")
		}
		if input.RepoSHAOut == "" {
			return CompleteJobResult{}, completeBadRequest("repo_sha_out is required for successful jobs with next_id")
		}
	}

	if input.Status == domaintypes.JobStatusSuccess {
		if err := maybeCloneSkippedStepDiffBeforeCompletion(ctx, s.store, s.blobpersist, job); err != nil {
			slog.Error("complete job: clone skipped step diff failed",
				"job_id", input.JobID,
				"repo_id", job.RepoID,
				"err", err,
			)
			return CompleteJobResult{}, completeInternal("failed to clone skipped step diff", err)
		}

		sbomRowsPersisted, sbomErr := maybePersistGateSuccessSBOMRows(ctx, s.store, s.blobpersist, job, input.Status)
		if sbomErr != nil {
			slog.Error("complete job: persist gate sbom rows failed",
				"job_id", input.JobID,
				"repo_id", job.RepoID,
				"job_type", job.JobType,
				"err", sbomErr,
			)
			return CompleteJobResult{}, completeInternal("failed to persist gate sbom rows", sbomErr)
		}
		if sbomRowsPersisted > 0 {
			slog.Info("complete job: persisted gate sbom rows",
				"job_id", input.JobID,
				"repo_id", job.RepoID,
				"job_type", job.JobType,
				"row_count", sbomRowsPersisted,
			)
		}
	}

	if input.StatsPayload.HasJobResources() {
		res := input.StatsPayload.JobResources
		if err := s.store.UpsertJobMetric(ctx, store.UpsertJobMetricParams{
			NodeID:            input.NodeID,
			JobID:             job.ID,
			CpuConsumedNs:     res.CPUConsumedNs,
			DiskConsumedBytes: res.DiskConsumedBytes,
			MemConsumedBytes:  res.MemConsumedBytes,
		}); err != nil {
			slog.Error("complete job: persist job metrics failed",
				"job_id", input.JobID,
				"node_id", input.NodeID,
				"err", err,
			)
			return CompleteJobResult{}, completeInternal("failed to persist job metrics", err)
		}
	}

	persistedMeta := append([]byte(nil), job.Meta...)
	if input.StatsPayload.HasJobMeta() {
		mergedMeta, mergeErr := mergeCompletionJobMeta(job.Meta, input.StatsPayload.JobMeta)
		if mergeErr != nil {
			slog.Error("complete job: merge metadata failed", "job_id", input.JobID, "err", mergeErr)
			return CompleteJobResult{}, completeInternal("failed to merge job metadata", mergeErr)
		}
		if err := s.store.UpdateJobCompletionWithMeta(ctx, store.UpdateJobCompletionWithMetaParams{
			ID:         job.ID,
			Status:     input.Status,
			ExitCode:   input.ExitCode,
			Meta:       mergedMeta,
			RepoShaOut: input.RepoSHAOut,
		}); err != nil {
			slog.Error("complete job: update failed",
				"job_id", input.JobID,
				"next_id", job.NextID,
				"node_id", input.NodeID,
				"err", err,
			)
			return CompleteJobResult{}, completeInternal("failed to complete job", err)
		}
		persistedMeta = mergedMeta
	} else {
		if err := s.store.UpdateJobCompletion(ctx, store.UpdateJobCompletionParams{
			ID:         job.ID,
			Status:     input.Status,
			ExitCode:   input.ExitCode,
			RepoShaOut: input.RepoSHAOut,
		}); err != nil {
			slog.Error("complete job: update failed",
				"job_id", input.JobID,
				"next_id", job.NextID,
				"node_id", input.NodeID,
				"err", err,
			)
			return CompleteJobResult{}, completeInternal("failed to complete job", err)
		}
	}

	slog.Info("job completed",
		"job_id", input.JobID,
		"next_id", job.NextID,
		"node_id", input.NodeID,
		"status", input.Status,
		"exit_code", input.ExitCode,
		"stats_size", len(input.StatsBytes),
	)

	// Emit retention hint followed by done sentinel on the job-scoped SSE
	// stream so clients receive log retention metadata before the stream closes.
	if s.eventsService != nil {
		if err := s.eventsService.PublishJobRetention(ctx, input.JobID, logstream.RetentionHint{
			Retained: true,
		}); err != nil {
			slog.Error("complete job: publish job retention failed",
				"job_id", input.JobID,
				"err", err,
			)
		}
		if err := s.eventsService.PublishJobDone(ctx, input.JobID, string(input.Status)); err != nil {
			slog.Error("complete job: publish job done failed",
				"job_id", input.JobID,
				"err", err,
			)
		}
	}

	state := &completeJobState{
		input:         input,
		job:           job,
		jobType:       jobType,
		serviceType:   serviceType,
		serviceTypeOK: serviceTypeOK,
		persistedMeta: persistedMeta,
	}

	s.onFail(ctx, state)
	s.onCancelled(ctx, state)
	s.onSuccess(ctx, state)
	s.reconcileRepoRun(ctx, state)
	s.mergeMRURL(ctx, state)

	return CompleteJobResult{}, nil
}

func (s *CompleteJobService) loadRunForPostCompletion(ctx context.Context, state *completeJobState, purpose string) (store.Run, bool) {
	if state.runCache.ok {
		return state.runCache.run, true
	}

	run, runErr := s.store.GetRun(ctx, state.job.RunID)
	if runErr == nil {
		state.runCache.run = run
		state.runCache.ok = true
		return run, true
	}

	slog.Warn("complete job: get run failed, retrying",
		"job_id", state.job.ID,
		"run_id", state.job.RunID,
		"purpose", purpose,
		"err", runErr,
	)
	run, retryErr := s.store.GetRun(ctx, state.job.RunID)
	if retryErr != nil {
		slog.Error("complete job: get run failed",
			"job_id", state.job.ID,
			"run_id", state.job.RunID,
			"purpose", purpose,
			"err", retryErr,
		)
		return store.Run{}, false
	}
	state.runCache.run = run
	state.runCache.ok = true
	return run, true
}
