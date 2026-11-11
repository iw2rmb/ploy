package lifecycle

import (
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// CompletionChainAction is the chain management action required after a job completes.
type CompletionChainAction int

const (
	// CompletionChainNoAction means no chain management is needed (e.g. success with no successor).
	CompletionChainNoAction CompletionChainAction = iota
	// CompletionChainCancelRemainder cancels remaining non-terminal jobs in the chain.
	CompletionChainCancelRemainder
	// CompletionChainAdvanceNext promotes the next linked job for execution.
	CompletionChainAdvanceNext
)

// CompletionDecision is the pure output of completion transition evaluation.
type CompletionDecision struct {
	ChainAction CompletionChainAction
}

// EvaluateCompletionDecision computes the chain management action required after a job
// completes. It is pure: no I/O is performed.
// hasNext should be true when the completed job has a linked successor (NextID != nil).
func EvaluateCompletionDecision(
	jobStatus domaintypes.JobStatus,
	hasNext bool,
) CompletionDecision {
	switch jobStatus {
	case domaintypes.JobStatusSuccess:
		if hasNext {
			return CompletionDecision{ChainAction: CompletionChainAdvanceNext}
		}
		return CompletionDecision{ChainAction: CompletionChainNoAction}
	case domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
		return CompletionDecision{ChainAction: CompletionChainCancelRemainder}
	default:
		return CompletionDecision{ChainAction: CompletionChainNoAction}
	}
}

// IsGateJobType reports whether jobType is a gate variant (pre or post).
func IsGateJobType(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate
}

// ChainPredecessor returns the job in jobsByID whose NextID points to jobID,
// or nil if no such job exists.
func ChainPredecessor(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}
