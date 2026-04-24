package lifecycle

import (
	"context"
	"errors"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ========== Execution Error Classification ==========

// JobStatusFromRunError maps a job execution error to the appropriate terminal job status.
// context.Canceled and context.DeadlineExceeded produce Cancelled; all other
// errors produce Error. This is the canonical status-from-error mapping consumed
// across all nodeagent execution paths.
func JobStatusFromRunError(err error) domaintypes.JobStatus {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domaintypes.JobStatusCancelled
	}
	return domaintypes.JobStatusError
}

// JobStatusFromExitCode maps a completed process exit code to terminal status.
// 0 => Success, 1 => Fail, any other value => Error.
func JobStatusFromExitCode(exitCode int) domaintypes.JobStatus {
	if exitCode == 0 {
		return domaintypes.JobStatusSuccess
	}
	if exitCode == 1 {
		return domaintypes.JobStatusFail
	}
	return domaintypes.JobStatusError
}

// JobStatusFromExitCodeForJobType maps process exit codes to terminal status with
// job-type-specific overrides.
func JobStatusFromExitCodeForJobType(jobType domaintypes.JobType, exitCode int) domaintypes.JobStatus {
	_ = jobType
	return JobStatusFromExitCode(exitCode)
}

// ========== Claim Decision ==========

// ClaimDecision is the pure output of claim transition evaluation.
type ClaimDecision struct {
	// AdvanceRunRepoToRunning is true when the RunRepo should be transitioned
	// from Queued to Running.
	AdvanceRunRepoToRunning bool
}

// EvaluateClaimDecision computes whether the RunRepo should advance to Running.
// Jobs advance the RunRepo from Queued to Running on first claim.
func EvaluateClaimDecision(jobType domaintypes.JobType, rrStatus domaintypes.RunRepoStatus) ClaimDecision {
	return ClaimDecision{
		AdvanceRunRepoToRunning: !jobType.IsZero() && rrStatus == domaintypes.RunRepoStatusQueued,
	}
}

// ========== Completion Decision ==========

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
	jobType domaintypes.JobType,
	jobStatus domaintypes.JobStatus,
	hasNext bool,
) CompletionDecision {
	_ = jobType
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

// ========== Recovery Context Resolution ==========

// ResolveGateRecoveryContext extracts recovery classification and stack detection
// from the failed gate job's metadata. Returns safe defaults when metadata is
// absent or unparseable.
func ResolveGateRecoveryContext(failedJob store.Job) (*contracts.BuildGateRecoveryMetadata, contracts.MigStack, *contracts.StackExpectation) {
	meta := &contracts.BuildGateRecoveryMetadata{
		LoopKind:  contracts.DefaultRecoveryLoopKind().String(),
		ErrorKind: contracts.DefaultRecoveryErrorKind().String(),
	}
	detectedStack := contracts.MigStackUnknown
	var detectedExpectation *contracts.StackExpectation

	if len(failedJob.Meta) == 0 {
		return meta, detectedStack, detectedExpectation
	}

	jobMeta, err := contracts.UnmarshalJobMeta(failedJob.Meta)
	if err != nil {
		return meta, detectedStack, detectedExpectation
	}

	if jobMeta.GateMetadata != nil {
		detectedStack = jobMeta.GateMetadata.DetectedStack()
		detectedExpectation = jobMeta.GateMetadata.DetectedStackExpectation()
		if jobMeta.GateMetadata.Recovery != nil {
			meta = CloneRecoveryMetadata(jobMeta.GateMetadata.Recovery)
		}
	}
	if detectedExpectation == nil {
		detectedExpectation = StackExpectationFromMigStack(detectedStack)
	}
	if kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind); (!ok || kind == contracts.RecoveryErrorKindUnknown) && jobMeta.RecoveryMetadata != nil {
		meta = CloneRecoveryMetadata(jobMeta.RecoveryMetadata)
	}
	if loopKind, ok := contracts.ParseRecoveryLoopKind(meta.LoopKind); ok {
		meta.LoopKind = loopKind.String()
	} else {
		meta.LoopKind = contracts.DefaultRecoveryLoopKind().String()
	}
	if kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind); ok {
		meta.ErrorKind = kind.String()
	} else {
		meta.ErrorKind = contracts.DefaultRecoveryErrorKind().String()
	}
	if meta.StrategyID == "" {
		meta.StrategyID = fmt.Sprintf("%s-default", meta.ErrorKind)
	}
	return meta, detectedStack, detectedExpectation
}

// RecoveryChainPredecessor returns the job in jobsByID whose NextID points to jobID,
// or nil if no such job exists.
func RecoveryChainPredecessor(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}

// ========== Internal helpers ==========

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// CloneRecoveryMetadata returns a deep copy of src, or nil if src is nil.
func CloneRecoveryMetadata(src *contracts.BuildGateRecoveryMetadata) *contracts.BuildGateRecoveryMetadata {
	if src == nil {
		return nil
	}
	out := *src
	out.Confidence = clonePtr(src.Confidence)
	out.CandidatePromoted = clonePtr(src.CandidatePromoted)
	if len(src.Expectations) > 0 {
		out.Expectations = append([]byte(nil), src.Expectations...)
	}
	if len(src.CandidateGateProfile) > 0 {
		out.CandidateGateProfile = append([]byte(nil), src.CandidateGateProfile...)
	}
	if src.DepsBumps != nil {
		out.DepsBumps = CloneDepsBumpsMap(src.DepsBumps)
	}
	return &out
}

// CloneDepsBumpsMap returns a deep copy of a dependency-bump version map.
func CloneDepsBumpsMap(src map[string]*string) map[string]*string {
	if src == nil {
		return nil
	}
	out := make(map[string]*string, len(src))
	for k, v := range src {
		if v == nil {
			out[k] = nil
			continue
		}
		ver := *v
		out[k] = &ver
	}
	return out
}

// StackExpectationFromMigStack converts a detected MigStack to a StackExpectation,
// or returns nil for unknown stacks.
func StackExpectationFromMigStack(stack contracts.MigStack) *contracts.StackExpectation {
	switch stack {
	case contracts.MigStackJavaMaven:
		return &contracts.StackExpectation{Language: "java", Tool: "maven"}
	case contracts.MigStackJavaGradle:
		return &contracts.StackExpectation{Language: "java", Tool: "gradle"}
	case contracts.MigStackJava:
		return &contracts.StackExpectation{Language: "java", Tool: "java"}
	default:
		return nil
	}
}
