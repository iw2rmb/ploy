package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// maybeCreateHealingJobs inserts a heal -> re-gate chain after a failed gate job by rewiring next_id links.
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
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

	recoveryMeta, detectedStack, detectedExpectation := resolveFailedGateRecoveryContext(failedJob)
	recoveryKind, ok := contracts.ParseRecoveryErrorKind(recoveryMeta.ErrorKind)
	if !ok {
		recoveryKind = contracts.DefaultRecoveryErrorKind()
	}
	if contracts.IsTerminalRecoveryErrorKind(recoveryKind) {
		slog.Info("maybeCreateHealingJobs: terminal recovery classification, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error_kind", recoveryMeta.ErrorKind,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
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
	if healing == nil || len(healing.ByErrorKind) == 0 {
		slog.Debug("maybeCreateHealingJobs: no healing config, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	action, ok := healing.ByErrorKind[recoveryKind.String()]
	if !ok {
		slog.Info("maybeCreateHealingJobs: no healing action for error_kind, canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error_kind", recoveryMeta.ErrorKind,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}
	if len(recoveryMeta.Expectations) == 0 && action.Expectations != nil {
		if b, err := json.Marshal(action.Expectations); err == nil {
			recoveryMeta.Expectations = b
		}
	}

	retries := action.Retries
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

	healImage, err := action.Image.ResolveImage(detectedStack)
	if err != nil {
		return fmt.Errorf("resolve healing image for stack %q: %w", detectedStack, err)
	}

	reGateRecoveryMeta := cloneRecoveryMetadata(recoveryMeta)
	if shouldEvaluateInfraCandidate(recoveryMeta, action) {
		if reGateRecoveryMeta == nil {
			reGateRecoveryMeta = &contracts.BuildGateRecoveryMetadata{
				LoopKind:  recoveryMeta.LoopKind,
				ErrorKind: recoveryMeta.ErrorKind,
			}
		}
		artifactPath := contracts.GateProfileCandidateArtifactPath
		if p, ok := resolveRecoveryCandidateArtifactPath(recoveryMeta.Expectations); ok {
			artifactPath = p
		}
		reGateRecoveryMeta.CandidateSchemaID = contracts.GateProfileCandidateSchemaID
		reGateRecoveryMeta.CandidateArtifactPath = artifactPath
		evaluateAndAttachInfraCandidate(
			ctx,
			bp,
			run.ID,
			failedJob,
			jobsByID,
			detectedExpectation,
			reGateRecoveryMeta,
		)
	}

	reGateMetaBytes, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:     contracts.JobKindGate,
		Recovery: reGateRecoveryMeta,
	})
	if err != nil {
		return fmt.Errorf("marshal re-gate job meta: %w", err)
	}
	healMetaBytes, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:     contracts.JobKindMod,
		Recovery: cloneRecoveryMetadata(recoveryMeta),
	})
	if err != nil {
		return fmt.Errorf("marshal heal job meta: %w", err)
	}

	oldNext := failedJob.NextID
	healID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()
	healRepoSHAIn := strings.TrimSpace(strings.ToLower(failedJob.RepoShaIn))
	if !sha40Pattern.MatchString(healRepoSHAIn) {
		slog.Error("maybeCreateHealingJobs: invalid failed job repo_sha_in; canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"repo_sha_in", failedJob.RepoShaIn,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
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
		Meta:        reGateMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

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
		Meta:        healMetaBytes,
		RepoShaIn:   healRepoSHAIn,
	})
	if err != nil {
		return fmt.Errorf("create heal job: %w", err)
	}

	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: failedJob.ID, NextID: &healID}); err != nil {
		return fmt.Errorf("rewire failed job next_id: %w", err)
	}

	slog.Info("maybeCreateHealingJobs: rewired chain",
		"run_id", failedJob.RunID,
		"failed_job_id", failedJob.ID,
		"heal_job_id", healID,
		"re_gate_job_id", reGateID,
		"old_next", oldNext,
		"attempt", healingAttemptNumber,
		"error_kind", recoveryMeta.ErrorKind,
		"strategy_id", recoveryMeta.StrategyID,
	)
	return nil
}

func cloneRecoveryMetadata(src *contracts.BuildGateRecoveryMetadata) *contracts.BuildGateRecoveryMetadata {
	if src == nil {
		return nil
	}
	out := *src
	if src.Confidence != nil {
		v := *src.Confidence
		out.Confidence = &v
	}
	if src.CandidatePromoted != nil {
		v := *src.CandidatePromoted
		out.CandidatePromoted = &v
	}
	if len(src.Expectations) > 0 {
		out.Expectations = append([]byte(nil), src.Expectations...)
	}
	if len(src.CandidateGateProfile) > 0 {
		out.CandidateGateProfile = append([]byte(nil), src.CandidateGateProfile...)
	}
	return &out
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
