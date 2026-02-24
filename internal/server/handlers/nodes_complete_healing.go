package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// maybeCreateHealingJobs inserts a heal -> re-gate chain after a failed gate job by rewiring next_id links.
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	run store.Run,
	failedJob store.Job,
) error {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}

	// Refresh failed job from storage snapshot if present.
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		failedJob = refreshed
	}

	jobType := domaintypes.JobType(failedJob.JobType)
	if err := jobType.Validate(); err != nil {
		return fmt.Errorf("invalid job_type %q for failed job_id=%s: %w", failedJob.JobType, failedJob.ID, err)
	}
	if !isGateJobType(jobType) {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"job_type", jobType.String(),
		)
		return nil
	}

	specRow, err := st.GetSpec(ctx, run.SpecID)
	if err != nil {
		return fmt.Errorf("get spec: %w", err)
	}
	spec, err := contracts.ParseModsSpecJSON(specRow.Spec)
	if err != nil {
		return fmt.Errorf("parse run spec: %w", err)
	}

	healing := (*contracts.HealingSpec)(nil)
	if spec.BuildGate != nil {
		healing = spec.BuildGate.Healing
	}
	if healing == nil || healing.Image.IsEmpty() {
		slog.Debug("maybeCreateHealingJobs: no healing config, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	retries := healing.Retries
	if retries <= 0 {
		retries = 1
	}

	baseGateID := resolveBaseGateID(failedJob, jobsByID)
	healingAttempts := countExistingHealingAttempts(baseGateID, jobsByID)
	healingAttemptNumber := healingAttempts + 1
	if healingAttemptNumber > retries {
		slog.Info("maybeCreateHealingJobs: healing retries exhausted",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"attempt", healingAttemptNumber,
			"max_retries", retries,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	healImage := ""
	if healing.Image.Universal != "" {
		healImage = strings.TrimSpace(healing.Image.Universal)
	}

	oldNext := failedJob.NextID
	healID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          healID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("heal-%d-0", healingAttemptNumber),
		JobType:     domaintypes.JobTypeHeal.String(),
		JobImage:    healImage,
		Status:      store.JobStatusQueued,
		NextID:      &reGateID,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create heal job: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          reGateID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("re-gate-%d", healingAttemptNumber),
		JobType:     domaintypes.JobTypeReGate.String(),
		JobImage:    "",
		Status:      store.JobStatusCreated,
		NextID:      oldNext,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: failedJob.ID, NextID: &healID}); err != nil {
		return fmt.Errorf("rewire failed job next_id: %w", err)
	}

	// Keep last inserted node's next pointer aligned with the captured old_next.
	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: reGateID, NextID: oldNext}); err != nil {
		return fmt.Errorf("rewire healing tail next_id: %w", err)
	}

	slog.Info("maybeCreateHealingJobs: rewired chain",
		"run_id", failedJob.RunID,
		"failed_job_id", failedJob.ID,
		"heal_job_id", healID,
		"re_gate_job_id", reGateID,
		"old_next", oldNext,
		"attempt", healingAttemptNumber,
	)
	return nil
}

func isGateJobType(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate
}

func predecessorOf(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}

func resolveBaseGateID(failedJob store.Job, jobsByID map[domaintypes.JobID]store.Job) domaintypes.JobID {
	failedType := domaintypes.JobType(failedJob.JobType)
	if failedType != domaintypes.JobTypeReGate {
		return failedJob.ID
	}

	currentID := failedJob.ID
	for range len(jobsByID) {
		prev := predecessorOf(currentID, jobsByID)
		if prev == nil {
			break
		}
		prevType := domaintypes.JobType(prev.JobType)
		if prevType == domaintypes.JobTypePreGate || prevType == domaintypes.JobTypePostGate {
			return prev.ID
		}
		currentID = prev.ID
	}
	return failedJob.ID
}

func countExistingHealingAttempts(baseGateID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) int {
	base, ok := jobsByID[baseGateID]
	if !ok {
		return 0
	}

	attempts := 0
	seen := map[domaintypes.JobID]struct{}{}
	nextID := base.NextID
	for nextID != nil {
		if _, dup := seen[*nextID]; dup {
			break
		}
		seen[*nextID] = struct{}{}

		job, ok := jobsByID[*nextID]
		if !ok {
			break
		}
		jobType := domaintypes.JobType(job.JobType)
		if jobType == domaintypes.JobTypeHeal {
			attempts++
		}
		if jobType != domaintypes.JobTypeHeal && jobType != domaintypes.JobTypeReGate {
			break
		}
		nextID = job.NextID
	}
	return attempts
}

// cancelRemainingJobsAfterFailure cancels non-terminal jobs reachable from the failed job's successor chain.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	failedJob store.Job,
) error {
	now := time.Now().UTC()

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}

	nextID := failedJob.NextID
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		nextID = refreshed.NextID
	}

	seen := map[domaintypes.JobID]struct{}{}
	for nextID != nil {
		if _, dup := seen[*nextID]; dup {
			break
		}
		seen[*nextID] = struct{}{}

		job, ok := jobsByID[*nextID]
		if !ok {
			break
		}
		nextID = job.NextID

		switch job.Status {
		case store.JobStatusSuccess, store.JobStatusFail, store.JobStatusCancelled:
			continue
		}

		startedAt := job.StartedAt
		var durationMs int64
		if job.StartedAt.Valid {
			durationMs = now.Sub(job.StartedAt.Time).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}

		finishedAt := pgtype.Timestamptz{Time: now, Valid: true}
		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     store.JobStatusCancelled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", job.ID, err)
		}

		slog.Info("canceled linked job after failure",
			"run_id", failedJob.RunID,
			"failed_job_id", failedJob.ID,
			"job_id", job.ID,
		)
	}

	return nil
}
