package handlers

import (
	"context"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func resolveClaimDetectedStack(
	ctx context.Context,
	st store.Store,
	job store.Job,
) (*contracts.StackExpectation, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return nil, err
	}
	return resolveDetectedStackExpectationFromJobs(job.ID, jobs), nil
}

func resolveDetectedStackExpectationFromJobs(
	currentJobID domaintypes.JobID,
	jobs []store.Job,
) *contracts.StackExpectation {
	if currentJobID.IsZero() || len(jobs) == 0 {
		return nil
	}
	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, j := range jobs {
		jobsByID[j.ID] = j
	}

	currentID := currentJobID
	for range len(jobsByID) {
		prev := lifecycle.RecoveryChainPredecessor(currentID, jobsByID)
		if prev == nil {
			return nil
		}
		currentID = prev.ID
		if !isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) || len(prev.Meta) == 0 {
			continue
		}
		meta, err := contracts.UnmarshalJobMeta(prev.Meta)
		if err != nil || meta.GateMetadata == nil {
			continue
		}
		exp := contracts.NormalizeStackExpectation(meta.GateMetadata.DetectedStackExpectation())
		if exp != nil {
			return exp
		}
	}
	return nil
}
