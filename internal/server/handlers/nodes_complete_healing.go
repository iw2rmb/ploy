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

// maybeCreateHealingJobs creates a single healing job and a re-gate job when a gate job fails.
// This is called when a gate job (pre_gate, post_gate, re_gate) completes with reason="build-gate".
//
// The function:
// 1. Finds the failed gate job by step_index
// 2. Verifies it's a gate job (pre_gate, post_gate, re_gate)
// 3. Checks if healing is configured in the run spec (requires single-mod schema)
// 4. Counts existing healing attempts to enforce retry limits
// 5. Creates one healing job and one re-gate job at intermediate step_index values
//
// Note: Legacy multi-strategy forms (strategies[] or mods[] at top level) are
// no longer supported. Callers must use the canonical single-mod schema for
// healing configuration.
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	run store.Run,
	runID domaintypes.RunID,
	repoID domaintypes.ModRepoID,
	attempt int32,
	failedStepIndex domaintypes.StepIndex,
) error {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	// Find the failed gate job by step_index.
	var failedJob *store.Job
	for i := range jobs {
		if jobStepIndex(jobs[i]) == failedStepIndex {
			failedJob = &jobs[i]
			break
		}
	}
	if failedJob == nil {
		slog.Debug("maybeCreateHealingJobs: no job found at step_index",
			"run_id", runID,
			"step_index", failedStepIndex,
		)
		return nil
	}

	// Only create healing for gate jobs.
	modType := domaintypes.ModType(failedJob.JobType)
	if err := modType.Validate(); err != nil {
		return fmt.Errorf("invalid mod_type %q for failed job_id=%s: %w", failedJob.JobType, failedJob.ID, err)
	}
	if modType != domaintypes.ModTypePreGate && modType != domaintypes.ModTypePostGate && modType != domaintypes.ModTypeReGate {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", runID,
			"mod_type", modType.String(),
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

	// Check if healing is configured.
	healing := (*contracts.HealingSpec)(nil)
	if spec.BuildGate != nil {
		healing = spec.BuildGate.Healing
	}
	if healing == nil || healing.Image.IsEmpty() {
		slog.Debug("maybeCreateHealingJobs: no healing config, canceling remaining jobs",
			"run_id", runID,
		)
		if err := cancelRemainingJobsAfterFailure(ctx, st, runID, repoID, attempt, failedStepIndex); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs when no healing configured",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
		return nil
	}

	// Get retry limit (default to 1 if not specified).
	retries := healing.Retries
	if retries <= 0 {
		retries = 1
	}

	// Determine the base gate index used to count healing attempts.
	// Healing attempts are counted per build gate (pre/post) independently.
	// For re-gate failures, associate the failure with the nearest preceding
	// pre_gate/post_gate so that all healing jobs between that gate and the
	// next non-gate/non-heal job share the same attempt counter.
	baseGateIndex := failedStepIndex
	if modType == domaintypes.ModTypeReGate {
		var (
			baseFound     bool
			baseStepIndex float64
		)
		for _, job := range jobs {
			mt := domaintypes.ModType(job.JobType)
			if err := mt.Validate(); err != nil {
				return fmt.Errorf("invalid mod_type %q for job_id=%s: %w", job.JobType, job.ID, err)
			}
			if mt != domaintypes.ModTypePreGate && mt != domaintypes.ModTypePostGate {
				continue
			}
			if float64(jobStepIndex(job)) > float64(failedStepIndex) {
				continue
			}
			if !baseFound || float64(jobStepIndex(job)) > baseStepIndex {
				baseFound = true
				baseStepIndex = float64(jobStepIndex(job))
			}
		}
		if baseFound {
			baseGateIndex = domaintypes.StepIndex(baseStepIndex)
		}
	}

	windowStart := float64(baseGateIndex)

	// Find the earliest non-healing, non-gate job after the base gate.
	// This bounds the healing window for this gate so that retries are
	// counted independently for each build gate.
	var (
		windowEnd     float64
		hasWindowEnd  bool
		isGateJobType = func(t domaintypes.ModType) bool {
			return t == domaintypes.ModTypePreGate || t == domaintypes.ModTypePostGate || t == domaintypes.ModTypeReGate
		}
	)
	for _, job := range jobs {
		if float64(jobStepIndex(job)) <= windowStart {
			continue
		}
		jt := domaintypes.ModType(job.JobType)
		if err := jt.Validate(); err != nil {
			return fmt.Errorf("invalid mod_type %q for job_id=%s: %w", job.JobType, job.ID, err)
		}
		if jt == domaintypes.ModTypeHeal {
			continue
		}
		if isGateJobType(jt) {
			// Gate jobs (pre/post/re) live inside the healing window and
			// must not terminate it.
			continue
		}
		if !hasWindowEnd || float64(jobStepIndex(job)) < windowEnd {
			hasWindowEnd = true
			windowEnd = float64(jobStepIndex(job))
		}
	}

	// Count existing healing attempts for this gate by counting "heal" jobs
	// whose step_index lies within (baseGateIndex, windowEnd).
	healingAttempts := 0
	for _, job := range jobs {
		jt := domaintypes.ModType(job.JobType)
		if err := jt.Validate(); err != nil {
			return fmt.Errorf("invalid mod_type %q for job_id=%s: %w", job.JobType, job.ID, err)
		}
		if jt != domaintypes.ModTypeHeal {
			continue
		}
		if float64(jobStepIndex(job)) <= windowStart {
			continue
		}
		if hasWindowEnd && float64(jobStepIndex(job)) >= windowEnd {
			continue
		}
		healingAttempts++
	}

	// Check if retries exhausted. Each attempt creates a single heal job and a
	// single re-gate job for this gate.
	healingAttemptNumber := healingAttempts + 1 // 1-based attempt number
	if healingAttemptNumber > retries {
		slog.Info("maybeCreateHealingJobs: healing retries exhausted",
			"run_id", runID,
			"attempt", healingAttemptNumber,
			"max_retries", retries,
		)

		// When healing retries are exhausted and the gate still fails, cancel
		// all remaining non-terminal jobs for the run so the control plane
		// can derive a terminal run state and avoid leaving mods/post-gate
		// jobs stranded in created/pending state.
		if err := cancelRemainingJobsAfterFailure(ctx, st, runID, repoID, attempt, failedStepIndex); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs after exhausted healing",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
		return nil
	}

	// Find the next job after the failed gate to calculate insertion range.
	nextStepIndex := float64(failedStepIndex) + 1000 // Default gap
	for _, job := range jobs {
		if float64(jobStepIndex(job)) > float64(failedStepIndex) {
			if float64(jobStepIndex(job)) < nextStepIndex {
				nextStepIndex = float64(jobStepIndex(job))
			}
		}
	}

	// Calculate step_index allocation for a single healing path:
	//   - heal job sits midway between failed gate and next mainline job
	//   - re-gate job sits three-quarters of the way to next mainline job
	gapSize := nextStepIndex - float64(failedStepIndex)
	if gapSize <= 0 {
		gapSize = 1000
	}

	healStepIndex := float64(failedStepIndex) + gapSize*0.5
	reGateStepIndex := float64(failedStepIndex) + gapSize*0.75

	slog.Info("maybeCreateHealingJobs: creating linear healing jobs",
		"run_id", runID,
		"failed_step_index", failedStepIndex,
		"next_step_index", nextStepIndex,
		"heal_step_index", healStepIndex,
		"re_gate_step_index", reGateStepIndex,
		"attempt", healingAttemptNumber,
	)

	modImage := ""
	if healing.Image.Universal != "" {
		modImage = strings.TrimSpace(healing.Image.Universal)
	}

	// Create a single healing job for this attempt.
	healJobName := fmt.Sprintf("heal-%d-0", healingAttemptNumber)
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     attempt,
		Name:        healJobName,
		JobType:     domaintypes.ModTypeHeal.String(),
		JobImage:    modImage,
		Status:      store.JobStatusQueued,
		NextID:      nil,
		Meta:        withStepIndexMeta([]byte(`{}`), domaintypes.StepIndex(healStepIndex)),
	})
	if err != nil {
		return fmt.Errorf("create healing job %s: %w", healJobName, err)
	}

	slog.Info("created healing job",
		"run_id", runID,
		"job_name", healJobName,
		"step_index", healStepIndex,
		"status", store.JobStatusQueued,
		"image", modImage,
	)

	// Create a single re-gate job for this attempt.
	reGateName := fmt.Sprintf("re-gate-%d", healingAttemptNumber)
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     attempt,
		Name:        reGateName,
		JobType:     domaintypes.ModTypeReGate.String(),
		JobImage:    "",
		Status:      store.JobStatusCreated,
		NextID:      nil,
		Meta:        withStepIndexMeta([]byte(`{}`), domaintypes.StepIndex(reGateStepIndex)),
	})
	if err != nil {
		return fmt.Errorf("create re-gate job %s: %w", reGateName, err)
	}

	slog.Info("created re-gate job",
		"run_id", runID,
		"job_name", reGateName,
		"step_index", reGateStepIndex,
	)

	return nil
}

// cancelRemainingJobsAfterFailure cancels all non-terminal jobs with
// step_index greater than the failed job's step_index. This is used after the
// system determines that no further progression is possible (e.g., healing
// retries exhausted, gate failure with no healing configured, or non-gate job
// failure) to avoid leaving jobs stranded in created/pending state.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.ModRepoID,
	attempt int32,
	failedStepIndex domaintypes.StepIndex,
) error {
	now := time.Now().UTC()

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return fmt.Errorf("list jobs for repo attempt: %w", err)
	}

	for _, job := range jobs {
		if jobStepIndex(job) <= failedStepIndex {
			continue
		}

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

		finishedAt := pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		}

		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     store.JobStatusCancelled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", job.ID, err)
		}

		slog.Info("canceled job after failure",
			"run_id", runID,
			"job_id", job.ID, // Job IDs are KSUID strings.
			"step_index", jobStepIndex(job),
		)
	}

	return nil
}
