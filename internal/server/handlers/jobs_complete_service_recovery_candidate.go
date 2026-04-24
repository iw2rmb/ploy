package handlers

import (
	"bytes"
	"context"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
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
	recovery := meta.RecoveryMetadata
	if recovery == nil && meta.GateMetadata != nil {
		recovery = meta.GateMetadata.Recovery
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

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   healJob.RunID,
		RepoID:  healJob.RepoID,
		Attempt: healJob.Attempt,
	})
	if err != nil {
		return err
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, item := range jobs {
		jobsByID[item.ID] = item
	}

	reGateJobID, ok := findNextReGateAfterJob(healJob, jobsByID)
	if !ok {
		return nil
	}
	reGateJob := jobsByID[reGateJobID]

	meta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		return nil
	}
	recovery := meta.RecoveryMetadata
	if recovery == nil && meta.GateMetadata != nil {
		recovery = meta.GateMetadata.Recovery
	}
	if recovery == nil {
		return nil
	}
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return nil
	}

	var detectedExpectation *contracts.StackExpectation
	if failedGate := findFailedGateForReGate(reGateJob.ID, jobsByID); failedGate != nil {
		_, _, detectedExpectation = lifecycle.ResolveGateRecoveryContext(*failedGate)
	}

	updatedRecovery := lifecycle.CloneRecoveryMetadata(recovery)
	evaluateAndAttachInfraCandidate(ctx, st, bp, reGateJob.RunID, reGateJob, jobsByID, detectedExpectation, updatedRecovery)
	if meta.RecoveryMetadata != nil {
		meta.RecoveryMetadata = updatedRecovery
	}
	if meta.GateMetadata != nil && meta.GateMetadata.Recovery != nil {
		meta.GateMetadata.Recovery = updatedRecovery
	}
	if meta.RecoveryMetadata == nil && (meta.GateMetadata == nil || meta.GateMetadata.Recovery == nil) {
		meta.RecoveryMetadata = updatedRecovery
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

func findNextReGateAfterJob(start store.Job, jobsByID map[domaintypes.JobID]store.Job) (domaintypes.JobID, bool) {
	if start.NextID == nil {
		return "", false
	}
	seen := make(map[domaintypes.JobID]struct{}, len(jobsByID))
	currentID := *start.NextID
	for {
		if _, exists := seen[currentID]; exists {
			return "", false
		}
		seen[currentID] = struct{}{}

		job, ok := jobsByID[currentID]
		if !ok {
			return "", false
		}
		jobType := domaintypes.JobType(job.JobType)
		if jobType == domaintypes.JobTypeReGate {
			return job.ID, true
		}
		if jobType != domaintypes.JobTypeSBOM {
			return "", false
		}
		if job.NextID == nil {
			return "", false
		}
		currentID = *job.NextID
	}
}

func findFailedGateForReGate(reGateID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	prev := lifecycle.RecoveryChainPredecessor(reGateID, jobsByID)
	for prev != nil {
		jobType := domaintypes.JobType(prev.JobType)
		switch jobType {
		case domaintypes.JobTypeSBOM:
			prev = lifecycle.RecoveryChainPredecessor(prev.ID, jobsByID)
		case domaintypes.JobTypeHeal:
			prev = lifecycle.RecoveryChainPredecessor(prev.ID, jobsByID)
			for prev != nil {
				switch domaintypes.JobType(prev.JobType) {
				case domaintypes.JobTypeSBOM:
					prev = lifecycle.RecoveryChainPredecessor(prev.ID, jobsByID)
				case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
					return prev
				default:
					return nil
				}
			}
			return nil
		case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
			return prev
		default:
			return nil
		}
	}
	return nil
}
