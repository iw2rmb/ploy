package handlers

import (
	"bytes"
	"context"
	"errors"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
	"github.com/jackc/pgx/v5"
)

func maybePromoteReGateRecoveryCandidate(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	job store.Job,
	rawMeta []byte,
) error {
	if bs == nil {
		return nil
	}
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeReGate {
		return nil
	}
	meta, err := contracts.UnmarshalJobMeta(rawMeta)
	if err != nil {
		return nil
	}
	recovery := meta.Recovery
	if recovery == nil && meta.Gate != nil {
		recovery = meta.Gate.Recovery
	}
	if recovery == nil {
		return nil
	}
	if recovery.CandidateValidationStatus != contracts.RecoveryCandidateStatusValid {
		return nil
	}
	if len(bytes.TrimSpace(recovery.CandidateGateProfile)) == 0 {
		return nil
	}
	if recovery.CandidatePromoted != nil && *recovery.CandidatePromoted {
		return nil
	}
	if err := persistReGateRecoveryCandidateProfile(ctx, st, bs, job, recovery); err != nil {
		return err
	}

	candidatePromoted := true
	recovery.CandidatePromoted = &candidatePromoted
	promotedMeta, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return err
	}
	return st.UpdateJobMeta(ctx, store.UpdateJobMetaParams{
		ID:   job.ID,
		Meta: promotedMeta,
	})
}

func maybeRefreshNextReGateRecoveryCandidate(
	ctx context.Context,
	st store.Store,
	bp *blobpersist.Service,
	healJob store.Job,
) error {
	if domaintypes.JobType(healJob.JobType) != domaintypes.JobTypeHeal {
		return nil
	}
	if bp == nil || healJob.NextID == nil {
		return nil
	}

	reGateJob, err := st.GetJob(ctx, *healJob.NextID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if domaintypes.JobType(reGateJob.JobType) != domaintypes.JobTypeReGate {
		return nil
	}

	meta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		return nil
	}
	recovery := meta.Recovery
	if recovery == nil && meta.Gate != nil {
		recovery = meta.Gate.Recovery
	}
	if recovery == nil {
		return nil
	}
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return nil
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   reGateJob.RunID,
		RepoID:  reGateJob.RepoID,
		Attempt: reGateJob.Attempt,
	})
	if err != nil {
		return err
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, item := range jobs {
		jobsByID[item.ID] = item
	}
	if refreshed, ok := jobsByID[reGateJob.ID]; ok {
		reGateJob = refreshed
	}

	var detectedExpectation *contracts.StackExpectation
	if prevHeal := lifecycle.RecoveryChainPredecessor(reGateJob.ID, jobsByID); prevHeal != nil {
		if failedGate := lifecycle.RecoveryChainPredecessor(prevHeal.ID, jobsByID); failedGate != nil {
			_, _, detectedExpectation = lifecycle.ResolveGateRecoveryContext(*failedGate)
		}
	}

	updatedRecovery := lifecycle.CloneRecoveryMetadata(recovery)
	evaluateAndAttachInfraCandidate(ctx, bp, reGateJob.RunID, reGateJob, jobsByID, detectedExpectation, updatedRecovery)
	if meta.Recovery != nil {
		meta.Recovery = updatedRecovery
	}
	if meta.Gate != nil && meta.Gate.Recovery != nil {
		meta.Gate.Recovery = updatedRecovery
	}
	if meta.Recovery == nil && (meta.Gate == nil || meta.Gate.Recovery == nil) {
		meta.Recovery = updatedRecovery
	}

	updatedMeta, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return err
	}
	return st.UpdateJobMeta(ctx, store.UpdateJobMetaParams{
		ID:   reGateJob.ID,
		Meta: updatedMeta,
	})
}
