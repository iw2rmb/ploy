package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// maybeCreateHealingJobs inserts a heal -> retry-sbom -> re-gate chain after a failed gate job by rewiring next_id links.
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
	if !lifecycle.IsGateJobType(jobType) {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"job_type", jobType.String(),
		)
		return nil
	}

	recoveryMeta, detectedStack, detectedExpectation := lifecycle.ResolveGateRecoveryContext(failedJob)

	// Fetch spec to evaluate the single heal/re-gate retry policy.
	specRow, err := st.GetSpec(ctx, run.SpecID)
	if err != nil {
		return fmt.Errorf("get spec: %w", err)
	}
	spec, err := contracts.ParseMigSpecJSON(specRow.Spec)
	if err != nil {
		return fmt.Errorf("parse run spec: %w", err)
	}

	var heal *contracts.HealSpec
	if spec.BuildGate != nil {
		heal = spec.BuildGate.Heal
	}

	decision, decisionErr := lifecycle.EvaluateGateFailureTransition(
		failedJob, jobsByID, recoveryMeta, detectedStack, heal, domaintypes.NewJobID)
	if decisionErr != nil {
		return fmt.Errorf("evaluate gate failure transition: %w", decisionErr)
	}

	if decision.Outcome == lifecycle.GateFailureOutcomeCancel {
		slog.Info("maybeCreateHealingJobs: canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error_kind", recoveryMeta.ErrorKind,
			"reason", decision.CancelReason,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}

	chain := decision.Chain

	// Attach infra candidate artifact if the orchestrator determined it is needed.
	if chain.ShouldAttachCandidate {
		evaluateAndAttachInfraCandidate(
			ctx, st, bp, run.ID, failedJob, jobsByID, detectedExpectation, chain.ReGateMeta.RecoveryMetadata)
	}

	reGateMetaBytes, err := contracts.MarshalJobMeta(chain.ReGateMeta)
	if err != nil {
		return fmt.Errorf("marshal re-gate job meta: %w", err)
	}
	healMetaBytes, err := contracts.MarshalJobMeta(chain.HealMeta)
	if err != nil {
		return fmt.Errorf("marshal heal job meta: %w", err)
	}

	reGateName := fmt.Sprintf("re-gate-%d", chain.AttemptNumber)

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          chain.ReGateID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        reGateName,
		JobType:     domaintypes.JobTypeReGate,
		JobImage:    "",
		Status:      domaintypes.JobStatusCreated,
		NextID:      chain.OldSuccessorID,
		Meta:        reGateMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

	retrySBOMMeta := contracts.NewMigJobMeta()
	retrySBOMMeta.SBOM = sbomCycleContextMeta(sbomCycleContext{
		Phase:     chain.RetrySBOMPhase,
		CycleName: reGateName,
		Role:      contracts.SBOMRoleRetry,
		RootJobID: chain.RetrySBOMRoot,
	})
	retrySBOMMetaBytes, err := contracts.MarshalJobMeta(retrySBOMMeta)
	if err != nil {
		return fmt.Errorf("marshal retry sbom job meta: %w", err)
	}
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          chain.RetrySBOMID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("sbom-retry-%d-%s", chain.AttemptNumber, chain.RetrySBOMID),
		JobType:     domaintypes.JobTypeSBOM,
		Status:      domaintypes.JobStatusCreated,
		NextID:      &chain.ReGateID,
		Meta:        retrySBOMMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create retry sbom job: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          chain.HealID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("heal-%d-0", chain.AttemptNumber),
		JobType:     domaintypes.JobTypeHeal,
		JobImage:    chain.HealImage,
		Status:      domaintypes.JobStatusQueued,
		NextID:      &chain.RetrySBOMID,
		Meta:        healMetaBytes,
		RepoShaIn:   chain.HealRepoSHAIn,
	})
	if err != nil {
		return fmt.Errorf("create heal job: %w", err)
	}

	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: failedJob.ID, NextID: &chain.HealID}); err != nil {
		return fmt.Errorf("rewire failed job next_id: %w", err)
	}
	healHead := store.Job{
		ID:      chain.HealID,
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
		JobType: domaintypes.JobTypeHeal,
		NextID:  &chain.RetrySBOMID,
	}
	if err := applyInsertedHeadRepoSHA(ctx, st, healHead, effectiveCompletedRepoSHAOut(failedJob, "")); err != nil {
		return fmt.Errorf("seed/clear repo sha for inserted healing chain: %w", err)
	}

	slog.Info("maybeCreateHealingJobs: rewired chain",
		"run_id", failedJob.RunID,
		"failed_job_id", failedJob.ID,
		"heal_job_id", chain.HealID,
		"re_gate_job_id", chain.ReGateID,
		"old_next", chain.OldSuccessorID,
		"attempt", chain.AttemptNumber,
		"error_kind", recoveryMeta.ErrorKind,
		"strategy_id", recoveryMeta.StrategyID,
	)
	return nil
}

func maybeCreateSBOMHealingJobs(
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
	if refreshed, ok := jobsByID[failedJob.ID]; ok {
		failedJob = refreshed
	}

	specRow, err := st.GetSpec(ctx, run.SpecID)
	if err != nil {
		return fmt.Errorf("get spec: %w", err)
	}
	spec, err := contracts.ParseMigSpecJSON(specRow.Spec)
	if err != nil {
		return fmt.Errorf("parse run spec: %w", err)
	}

	var heal *contracts.HealSpec
	if spec.BuildGate != nil {
		heal = spec.BuildGate.Heal
	}
	decision, decisionErr := lifecycle.EvaluateSBOMFailureTransition(
		failedJob,
		jobsByID,
		heal,
		contracts.MigStackUnknown,
		domaintypes.NewJobID,
	)
	if decisionErr != nil {
		return fmt.Errorf("evaluate sbom failure transition: %w", decisionErr)
	}
	if decision.Outcome == lifecycle.SBOMFailureOutcomeCancel {
		slog.Info("maybeCreateSBOMHealingJobs: canceling remaining linked jobs",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"reason", decision.CancelReason,
		)
		return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
	}
	chain := decision.Chain
	healMeta := contracts.NewMigJobMeta()
	healMetaBytes, err := contracts.MarshalJobMeta(healMeta)
	if err != nil {
		return fmt.Errorf("marshal heal job meta: %w", err)
	}
	failedCtx, _ := sbomCycleContextFromJob(failedJob)
	retrySBOMMeta := contracts.NewMigJobMeta()
	retrySBOMMeta.SBOM = sbomCycleContextMeta(sbomCycleContext{
		Phase:     failedCtx.Phase,
		CycleName: sbomCycleNameFromContext(failedCtx),
		Role:      contracts.SBOMRoleRetry,
		RootJobID: chain.RootSBOMID,
	})
	retrySBOMMetaBytes, err := contracts.MarshalJobMeta(retrySBOMMeta)
	if err != nil {
		return fmt.Errorf("marshal retry sbom job meta: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          chain.RetrySBOMID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("sbom-retry-%d-%s", chain.AttemptNumber, chain.RetrySBOMID),
		JobType:     domaintypes.JobTypeSBOM,
		Status:      domaintypes.JobStatusCreated,
		NextID:      chain.OldSuccessorID,
		Meta:        retrySBOMMetaBytes,
	})
	if err != nil {
		return fmt.Errorf("create retry sbom job: %w", err)
	}
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          chain.HealID,
		RunID:       failedJob.RunID,
		RepoID:      failedJob.RepoID,
		RepoBaseRef: failedJob.RepoBaseRef,
		Attempt:     failedJob.Attempt,
		Name:        fmt.Sprintf("heal-sbom-%d-0", chain.AttemptNumber),
		JobType:     domaintypes.JobTypeHeal,
		JobImage:    chain.HealImage,
		Status:      domaintypes.JobStatusQueued,
		NextID:      &chain.RetrySBOMID,
		Meta:        healMetaBytes,
		RepoShaIn:   strings.TrimSpace(chain.HealRepoSHAIn),
	})
	if err != nil {
		return fmt.Errorf("create heal job: %w", err)
	}
	if err := st.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: failedJob.ID, NextID: &chain.HealID}); err != nil {
		return fmt.Errorf("rewire failed sbom next_id: %w", err)
	}
	healHead := store.Job{
		ID:      chain.HealID,
		RunID:   failedJob.RunID,
		RepoID:  failedJob.RepoID,
		Attempt: failedJob.Attempt,
		JobType: domaintypes.JobTypeHeal,
		NextID:  &chain.RetrySBOMID,
	}
	if err := applyInsertedHeadRepoSHA(ctx, st, healHead, effectiveCompletedRepoSHAOut(failedJob, "")); err != nil {
		return fmt.Errorf("seed/clear repo sha for inserted sbom-healing chain: %w", err)
	}
	return nil
}

func blobStoreForPlanning(bp *blobpersist.Service) blobstore.Store {
	if bp == nil {
		return nil
	}
	return bp.BlobStore()
}
