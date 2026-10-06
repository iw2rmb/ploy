package handlers

import (
	"context"
	"errors"
	"log/slog"
	"slices"

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
	input         completionInput
	job           store.Job
	jobType       domaintypes.JobType
	persistedMeta []byte
	runCache      completeRunCache
}

func (s *completionService) Complete(ctx context.Context, input completionInput) error {
	var completedRun *store.Run
	err := s.store.WithJobExecution(ctx, input.JobID, input.StatsPayload.ResumeCount, func(st store.Store, job store.Job) error {
		scoped := *s
		scoped.store = st
		var err error
		completedRun, err = scoped.complete(ctx, input, job)
		return err
	})
	if errors.Is(err, store.ErrJobExecutionStale) {
		return completeConflict("completion belongs to an earlier execution")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return completeNotFound("job not found")
	}
	if err == nil && completedRun != nil {
		s.reconcileWave(ctx, *completedRun)
	}
	return err
}

func (s *completionService) complete(ctx context.Context, input completionInput, job store.Job) (*store.Run, error) {

	if job.NodeID == nil || *job.NodeID != input.NodeID {
		return nil, completeForbidden("job not assigned to this node")
	}
	if job.Status != domaintypes.JobStatusRunning {
		return nil, completeConflict("job status is %s, expected Running", job.Status)
	}
	jobType := domaintypes.JobType(job.JobType)
	if err := jobType.Validate(); err != nil {
		slog.Error("complete job: invalid job_type in job record; treating as non-gate for post-completion routing",
			"job_id", input.JobID,
			"job_type", job.JobType,
		)
	}

	if input.Status == domaintypes.JobStatusSuccess {
		input.RepoSHAOut = normalizeRepoSHA(input.RepoSHAOut)
		if input.RepoSHAOut == "" && isNonChangingJob(jobType) {
			input.RepoSHAOut = normalizeRepoSHA(job.RepoShaIn)
		}
	}

	if input.Status == domaintypes.JobStatusSuccess && job.NextID != nil {
		if !domaintypes.IsCanonicalFullCommitSHA(job.RepoShaIn) {
			return nil, completeConflict("job repo_sha_in must match ^[0-9a-f]{40}$ for chain progression")
		}
		if input.RepoSHAOut == "" {
			return nil, completeBadRequest("repo_sha_out is required for successful jobs with next_id")
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
			return nil, completeInternal("failed to persist job metrics", err)
		}
	}

	persistedMeta := slices.Clone(job.Meta)
	if input.StatsPayload.HasJobMeta() {
		if err := input.StatsPayload.ValidateJobMeta(); err != nil {
			slog.Error("complete job: invalid metadata", "job_id", input.JobID, "err", err)
			return nil, completeInternal("invalid job metadata", err)
		}
		if err := s.store.UpdateJobCompletionWithMeta(ctx, store.UpdateJobCompletionWithMetaParams{
			ID:         job.ID,
			Status:     input.Status,
			ExitCode:   input.ExitCode,
			Meta:       input.StatsPayload.JobMeta,
			RepoShaOut: input.RepoSHAOut,
		}); err != nil {
			slog.Error("complete job: update failed",
				"job_id", input.JobID,
				"next_id", job.NextID,
				"node_id", input.NodeID,
				"err", err,
			)
			return nil, completeInternal("failed to complete job", err)
		}
		persistedMeta = input.StatsPayload.JobMeta
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
			return nil, completeInternal("failed to complete job", err)
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
		s.eventsService.Hub().ResumeJob(input.JobID, input.StatsPayload.ResumeCount)
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
		persistedMeta: persistedMeta,
	}

	s.onFail(ctx, state)
	s.onCancelled(ctx, state)
	s.onSuccess(ctx, state)
	return s.reconcileRepoRun(ctx, state), nil
}

func (s *completionService) loadRunForPostCompletion(ctx context.Context, state *completeJobState, purpose string) (store.Run, bool) {
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
