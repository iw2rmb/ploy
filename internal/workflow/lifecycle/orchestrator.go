package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ========== Execution Error Classification ==========

// JobStatusFromRunError maps a job execution error to the appropriate terminal job status.
// context.Canceled and context.DeadlineExceeded produce Cancelled; all other
// errors produce Fail. This is the canonical status-from-error mapping consumed
// across all nodeagent execution paths.
func JobStatusFromRunError(err error) domaintypes.JobStatus {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return domaintypes.JobStatusCancelled
	}
	return domaintypes.JobStatusFail
}

var sha40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ========== Claim Decision ==========

// ClaimDecision is the pure output of claim transition evaluation.
type ClaimDecision struct {
	// AdvanceRunRepoToRunning is true when the RunRepo should be transitioned
	// from Queued to Running.
	AdvanceRunRepoToRunning bool
}

// EvaluateClaimDecision computes whether the RunRepo should advance to Running.
// Non-MR jobs advance the RunRepo from Queued to Running on first claim.
func EvaluateClaimDecision(jobType domaintypes.JobType, rrStatus domaintypes.RunRepoStatus) ClaimDecision {
	return ClaimDecision{
		AdvanceRunRepoToRunning: jobType != domaintypes.JobTypeMR && rrStatus == domaintypes.RunRepoStatusQueued,
	}
}

// ========== Completion Decision ==========

// CompletionChainAction is the chain management action required after a job completes.
type CompletionChainAction int

const (
	// CompletionChainNoAction means no chain management is needed (e.g. MR jobs, success with no successor).
	CompletionChainNoAction CompletionChainAction = iota
	// CompletionChainCancelRemainder cancels remaining non-terminal jobs in the chain.
	CompletionChainCancelRemainder
	// CompletionChainAdvanceNext promotes the next linked job for execution.
	CompletionChainAdvanceNext
	// CompletionChainEvaluateGateFailure requires full gate failure evaluation (cancel or insert heal chain).
	CompletionChainEvaluateGateFailure
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
	switch jobStatus {
	case domaintypes.JobStatusSuccess:
		if hasNext {
			return CompletionDecision{ChainAction: CompletionChainAdvanceNext}
		}
		return CompletionDecision{ChainAction: CompletionChainNoAction}
	case domaintypes.JobStatusFail, domaintypes.JobStatusCancelled:
		if jobType == domaintypes.JobTypeMR {
			return CompletionDecision{ChainAction: CompletionChainNoAction}
		}
		if jobStatus == domaintypes.JobStatusFail && IsGateJobType(jobType) {
			return CompletionDecision{ChainAction: CompletionChainEvaluateGateFailure}
		}
		return CompletionDecision{ChainAction: CompletionChainCancelRemainder}
	default:
		return CompletionDecision{ChainAction: CompletionChainNoAction}
	}
}

// ========== Gate Failure Transition ==========

// IsGateJobType reports whether jobType is a gate variant (pre, post, or re-gate).
func IsGateJobType(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate || jobType == domaintypes.JobTypePostGate || jobType == domaintypes.JobTypeReGate
}

// GateFailureOutcome is the kind of action required after a gate job fails.
type GateFailureOutcome int

const (
	// GateFailureOutcomeCancel cancels remaining jobs in the successor chain.
	GateFailureOutcomeCancel GateFailureOutcome = iota
	// GateFailureOutcomeHealChain inserts a heal + re-gate job chain.
	GateFailureOutcomeHealChain
)

// HealChainSpec contains the parameters for creating a heal + re-gate job pair.
// When ShouldAttachCandidate is true, the caller must invoke the infra-candidate
// enrichment on ReGateMeta.Recovery before marshaling ReGateMeta.
type HealChainSpec struct {
	HealID        domaintypes.JobID
	ReGateID      domaintypes.JobID
	AttemptNumber int
	HealImage     string
	HealRepoSHAIn string
	OldSuccessorID *domaintypes.JobID
	// HealMeta is the unmaterialized job meta for the heal job.
	HealMeta *contracts.JobMeta
	// ReGateMeta is the unmaterialized job meta for the re-gate job.
	// If ShouldAttachCandidate is true, the caller must enrich
	// ReGateMeta.Recovery with the infra candidate artifact before marshaling.
	ReGateMeta *contracts.JobMeta
	// ShouldAttachCandidate indicates the caller must evaluate and attach the
	// infra candidate artifact to ReGateMeta.Recovery before marshaling.
	ShouldAttachCandidate bool
}

// GateFailureDecision is the pure result of gate failure transition evaluation.
type GateFailureDecision struct {
	Outcome      GateFailureOutcome
	CancelReason string        // non-empty when Outcome == GateFailureOutcomeCancel
	Chain        *HealChainSpec // non-nil when Outcome == GateFailureOutcomeHealChain
}

func cancelDecision(reason string) (GateFailureDecision, error) {
	return GateFailureDecision{Outcome: GateFailureOutcomeCancel, CancelReason: reason}, nil
}

// EvaluateGateFailureTransition computes what to do after a gate job fails.
// It is pure: all inputs are pre-loaded and no I/O is performed.
// newJobID is called to generate IDs for heal and re-gate jobs; pass
// domaintypes.NewJobID in production and a deterministic stub in tests.
func EvaluateGateFailureTransition(
	failedJob store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
	recoveryMeta *contracts.BuildGateRecoveryMetadata,
	recoveryKind contracts.RecoveryErrorKind,
	detectedStack contracts.MigStack,
	healing *contracts.HealingSpec,
	newJobID func() domaintypes.JobID,
) (GateFailureDecision, error) {
	if contracts.IsTerminalRecoveryErrorKind(recoveryKind) {
		return cancelDecision("terminal recovery classification")
	}

	if healing == nil || len(healing.ByErrorKind) == 0 {
		return cancelDecision("no healing config")
	}

	action, ok := healing.ByErrorKind[recoveryKind.String()]
	if !ok {
		return cancelDecision(fmt.Sprintf("no healing action for error_kind %q", recoveryKind))
	}

	retries := action.Retries
	if retries <= 0 {
		retries = 1
	}

	baseGateID := resolveBaseGateID(failedJob, jobsByID)
	healingAttempts := countExistingHealingAttempts(baseGateID, jobsByID)
	attemptNumber := healingAttempts + 1
	if attemptNumber > retries {
		return cancelDecision("healing retries exhausted")
	}

	healImage, err := action.Image.ResolveImage(detectedStack)
	if err != nil {
		return GateFailureDecision{}, fmt.Errorf("resolve healing image for stack %q: %w", detectedStack, err)
	}

	healRepoSHAIn := strings.TrimSpace(strings.ToLower(failedJob.RepoShaIn))
	if !sha40Pattern.MatchString(healRepoSHAIn) {
		return cancelDecision("invalid failed job repo_sha_in")
	}

	// Enrich recovery meta expectations from action spec if not already set.
	enrichedMeta := CloneRecoveryMetadata(recoveryMeta)
	if len(enrichedMeta.Expectations) == 0 && action.Expectations != nil {
		if b, marshalErr := json.Marshal(action.Expectations); marshalErr == nil {
			enrichedMeta.Expectations = b
		}
	}

	reGateRecoveryMeta := CloneRecoveryMetadata(enrichedMeta)
	shouldAttachCandidate := shouldEvaluateInfraCandidate(enrichedMeta, action)
	if shouldAttachCandidate {
		artifactPath := contracts.GateProfileCandidateArtifactPath
		if p, resolved := resolveRecoveryCandidateArtifactPath(enrichedMeta.Expectations); resolved {
			artifactPath = p
		}
		reGateRecoveryMeta.CandidateSchemaID = contracts.GateProfileCandidateSchemaID
		reGateRecoveryMeta.CandidateArtifactPath = artifactPath
	}

	healID := newJobID()
	reGateID := newJobID()

	return GateFailureDecision{
		Outcome: GateFailureOutcomeHealChain,
		Chain: &HealChainSpec{
			HealID:         healID,
			ReGateID:       reGateID,
			AttemptNumber:  attemptNumber,
			HealImage:      healImage,
			HealRepoSHAIn:  healRepoSHAIn,
			OldSuccessorID: failedJob.NextID,
			HealMeta: &contracts.JobMeta{
				Kind:     contracts.JobKindMod,
				Recovery: CloneRecoveryMetadata(enrichedMeta),
			},
			ReGateMeta: &contracts.JobMeta{
				Kind:     contracts.JobKindGate,
				Recovery: reGateRecoveryMeta,
			},
			ShouldAttachCandidate: shouldAttachCandidate,
		},
	}, nil
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

	if jobMeta.Gate != nil {
		detectedStack = jobMeta.Gate.DetectedStack()
		detectedExpectation = jobMeta.Gate.DetectedStackExpectation()
		if jobMeta.Gate.Recovery != nil {
			meta = CloneRecoveryMetadata(jobMeta.Gate.Recovery)
		}
	}
	if detectedExpectation == nil {
		detectedExpectation = StackExpectationFromMigStack(detectedStack)
	}
	if kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind); (!ok || kind == contracts.RecoveryErrorKindUnknown) && jobMeta.Recovery != nil {
		meta = CloneRecoveryMetadata(jobMeta.Recovery)
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

func resolveBaseGateID(failedJob store.Job, jobsByID map[domaintypes.JobID]store.Job) domaintypes.JobID {
	failedType := domaintypes.JobType(failedJob.JobType)
	if failedType != domaintypes.JobTypeReGate {
		return failedJob.ID
	}

	currentID := failedJob.ID
	for range len(jobsByID) {
		prev := RecoveryChainPredecessor(currentID, jobsByID)
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

func shouldEvaluateInfraCandidate(
	recoveryMeta *contracts.BuildGateRecoveryMetadata,
	action contracts.HealingActionSpec,
) bool {
	if recoveryMeta == nil {
		return false
	}
	kind, ok := contracts.ParseRecoveryErrorKind(recoveryMeta.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return false
	}
	if action.Expectations == nil {
		return false
	}
	for _, artifact := range action.Expectations.Artifacts {
		if strings.TrimSpace(artifact.Schema) == contracts.GateProfileCandidateSchemaID {
			return true
		}
	}
	return false
}

func resolveRecoveryCandidateArtifactPath(expectations json.RawMessage) (string, bool) {
	if len(expectations) == 0 {
		return "", false
	}
	var ex struct {
		Artifacts []struct {
			Path   string `json:"path"`
			Schema string `json:"schema"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(expectations, &ex); err != nil {
		return "", false
	}
	for _, artifact := range ex.Artifacts {
		if strings.TrimSpace(artifact.Schema) != contracts.GateProfileCandidateSchemaID {
			continue
		}
		path := strings.TrimSpace(artifact.Path)
		if path == "" {
			continue
		}
		return path, true
	}
	return "", false
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
