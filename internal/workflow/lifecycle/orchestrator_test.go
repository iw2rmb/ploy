package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// ========== EvaluateClaimDecision ==========

func TestEvaluateClaimDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		jobType     domaintypes.JobType
		rrStatus    domaintypes.RunRepoStatus
		wantAdvance bool
	}{
		{
			name:        "non-MR queued repo advances to running",
			jobType:     domaintypes.JobTypeMig,
			rrStatus:    domaintypes.RunRepoStatusQueued,
			wantAdvance: true,
		},
		{
			name:        "MR job does not advance repo",
			jobType:     domaintypes.JobTypeMR,
			rrStatus:    domaintypes.RunRepoStatusQueued,
			wantAdvance: false,
		},
		{
			name:        "non-MR running repo is not re-advanced",
			jobType:     domaintypes.JobTypeMig,
			rrStatus:    domaintypes.RunRepoStatusRunning,
			wantAdvance: false,
		},
		{
			name:        "pre-gate queued repo advances to running",
			jobType:     domaintypes.JobTypePreGate,
			rrStatus:    domaintypes.RunRepoStatusQueued,
			wantAdvance: true,
		},
		{
			name:        "pre-gate non-queued repo is not advanced",
			jobType:     domaintypes.JobTypePreGate,
			rrStatus:    domaintypes.RunRepoStatusSuccess,
			wantAdvance: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decision := lifecycle.EvaluateClaimDecision(tc.jobType, tc.rrStatus)
			if decision.AdvanceRunRepoToRunning != tc.wantAdvance {
				t.Fatalf("AdvanceRunRepoToRunning = %v, want %v", decision.AdvanceRunRepoToRunning, tc.wantAdvance)
			}
		})
	}
}

// ========== EvaluateCompletionDecision ==========

func TestEvaluateCompletionDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		jobType    domaintypes.JobType
		jobStatus  domaintypes.JobStatus
		hasNext    bool
		wantAction lifecycle.CompletionChainAction
	}{
		// Success paths
		{
			name:       "success with successor advances chain",
			jobType:    domaintypes.JobTypeMig,
			jobStatus:  domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:       "success without successor takes no action",
			jobType:    domaintypes.JobTypeMig,
			jobStatus:  domaintypes.JobStatusSuccess,
			hasNext:    false,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		// Fail paths
		{
			name:       "failed MR job takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:       "failed pre-gate triggers gate failure evaluation",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "failed post-gate triggers gate failure evaluation",
			jobType:    domaintypes.JobTypePostGate,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    false,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "failed re-gate triggers gate failure evaluation",
			jobType:    domaintypes.JobTypeReGate,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "failed non-gate mig job cancels chain",
			jobType:    domaintypes.JobTypeMig,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "failed heal job cancels chain",
			jobType:    domaintypes.JobTypeHeal,
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "failed unknown job type cancels chain",
			jobType:    "",
			jobStatus:  domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "errored gate job cancels remainder without gate failure evaluation",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusError,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "errored MR job takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			jobStatus:  domaintypes.JobStatusError,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		// Cancelled paths
		{
			name:       "cancelled MR job takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			jobStatus:  domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:       "cancelled non-MR job cancels remainder",
			jobType:    domaintypes.JobTypeMig,
			jobStatus:  domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "cancelled gate job cancels remainder",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := lifecycle.EvaluateCompletionDecision(tc.jobType, tc.jobStatus, tc.hasNext)
			if got.ChainAction != tc.wantAction {
				t.Fatalf("ChainAction = %v, want %v", got.ChainAction, tc.wantAction)
			}
		})
	}
}

// ========== IsGateJobType ==========

func TestIsGateJobType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		jobType domaintypes.JobType
		want    bool
	}{
		{domaintypes.JobTypePreGate, true},
		{domaintypes.JobTypePostGate, true},
		{domaintypes.JobTypeReGate, true},
		{domaintypes.JobTypeMig, false},
		{domaintypes.JobTypeHeal, false},
		{domaintypes.JobTypeMR, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.jobType), func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.IsGateJobType(tc.jobType); got != tc.want {
				t.Fatalf("IsGateJobType(%q) = %v, want %v", tc.jobType, got, tc.want)
			}
		})
	}
}

// ========== EvaluateGateFailureTransition ==========

const testRepoSHAIn = "0123456789abcdef0123456789abcdef01234567"

func newFixedIDSequence(ids ...domaintypes.JobID) func() domaintypes.JobID {
	i := 0
	return func() domaintypes.JobID {
		if i >= len(ids) {
			panic("newFixedIDSequence: ran out of ids")
		}
		id := ids[i]
		i++
		return id
	}
}

func basicHealSpec(retries int) *contracts.HealSpec {
	return &contracts.HealSpec{
		Retries: retries,
		Image:   contracts.JobImage{Universal: "heal:latest"},
	}
}

type gateFailureCase struct {
	name          string
	failedJob     store.Job
	jobsByID      map[domaintypes.JobID]store.Job
	recoveryMeta  *contracts.BuildGateRecoveryMetadata
	detectedStack contracts.MigStack
	heal          *contracts.HealSpec
	newJobID      func() domaintypes.JobID
	wantOutcome   lifecycle.GateFailureOutcome
	assertChain   func(*testing.T, *lifecycle.HealChainSpec)
}

func retriesExhaustedCase() gateFailureCase {
	baseGateID := domaintypes.NewJobID()
	heal1ID := domaintypes.NewJobID()
	reGate1ID := domaintypes.NewJobID()
	return gateFailureCase{
		name: "retries exhausted cancels",
		failedJob: store.Job{
			ID: reGate1ID, JobType: domaintypes.JobTypeReGate, RepoShaIn: testRepoSHAIn,
		},
		jobsByID: map[domaintypes.JobID]store.Job{
			baseGateID: {ID: baseGateID, JobType: domaintypes.JobTypePreGate, NextID: &heal1ID},
			heal1ID:    {ID: heal1ID, JobType: domaintypes.JobTypeHeal, NextID: &reGate1ID},
			reGate1ID:  {ID: reGate1ID, JobType: domaintypes.JobTypeReGate},
		},
		recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		heal:         basicHealSpec(1),
		newJobID:     domaintypes.NewJobID,
		wantOutcome:  lifecycle.GateFailureOutcomeCancel,
	}
}

func firstAttemptCase() gateFailureCase {
	baseGateID := domaintypes.NewJobID()
	successorID := domaintypes.NewJobID()
	healID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()
	failedJob := store.Job{
		ID: baseGateID, JobType: domaintypes.JobTypePreGate,
		RepoShaIn: testRepoSHAIn, NextID: &successorID,
	}
	return gateFailureCase{
		name:         "first attempt creates chain",
		failedJob:    failedJob,
		jobsByID:     map[domaintypes.JobID]store.Job{baseGateID: failedJob},
		recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "infra", StrategyID: "infra-default"},
		heal:         basicHealSpec(2),
		newJobID:     newFixedIDSequence(healID, reGateID),
		wantOutcome:  lifecycle.GateFailureOutcomeHealChain,
		assertChain: func(t *testing.T, chain *lifecycle.HealChainSpec) {
			t.Helper()
			if chain.HealID != healID {
				t.Fatalf("HealID = %s, want %s", chain.HealID, healID)
			}
			if chain.ReGateID != reGateID {
				t.Fatalf("ReGateID = %s, want %s", chain.ReGateID, reGateID)
			}
			if chain.AttemptNumber != 1 {
				t.Fatalf("AttemptNumber = %d, want 1", chain.AttemptNumber)
			}
			if chain.HealImage != "heal:latest" {
				t.Fatalf("HealImage = %q, want %q", chain.HealImage, "heal:latest")
			}
			if chain.HealRepoSHAIn != testRepoSHAIn {
				t.Fatalf("HealRepoSHAIn = %q, want %q", chain.HealRepoSHAIn, testRepoSHAIn)
			}
			if chain.OldSuccessorID == nil || *chain.OldSuccessorID != successorID {
				t.Fatalf("OldSuccessorID = %v, want %s", chain.OldSuccessorID, successorID)
			}
			if chain.ReGateMeta == nil || chain.ReGateMeta.RecoveryMetadata == nil {
				t.Fatal("expected ReGateMeta.RecoveryMetadata to be set")
			}
			if chain.ReGateMeta.RecoveryMetadata.ErrorKind != "infra" {
				t.Fatalf("ReGateMeta.RecoveryMetadata.ErrorKind = %q, want %q", chain.ReGateMeta.RecoveryMetadata.ErrorKind, "infra")
			}
			if chain.HealMeta == nil || chain.HealMeta.RecoveryMetadata == nil {
				t.Fatal("expected HealMeta.RecoveryMetadata to be set")
			}
		},
	}
}

func secondAttemptCase() gateFailureCase {
	baseGateID := domaintypes.NewJobID()
	heal1ID := domaintypes.NewJobID()
	reGate1ID := domaintypes.NewJobID()
	successorID := domaintypes.NewJobID()
	heal2ID := domaintypes.NewJobID()
	reGate2ID := domaintypes.NewJobID()
	return gateFailureCase{
		name: "second attempt increases number",
		failedJob: store.Job{
			ID: reGate1ID, JobType: domaintypes.JobTypeReGate,
			RepoShaIn: testRepoSHAIn, NextID: &successorID,
		},
		jobsByID: map[domaintypes.JobID]store.Job{
			baseGateID:  {ID: baseGateID, JobType: domaintypes.JobTypePreGate, NextID: &heal1ID},
			heal1ID:     {ID: heal1ID, JobType: domaintypes.JobTypeHeal, NextID: &reGate1ID},
			reGate1ID:   {ID: reGate1ID, JobType: domaintypes.JobTypeReGate, NextID: &successorID},
			successorID: {ID: successorID, JobType: domaintypes.JobTypeMig},
		},
		recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		heal:         basicHealSpec(3),
		newJobID:     newFixedIDSequence(heal2ID, reGate2ID),
		wantOutcome:  lifecycle.GateFailureOutcomeHealChain,
		assertChain: func(t *testing.T, chain *lifecycle.HealChainSpec) {
			t.Helper()
			if chain.AttemptNumber != 2 {
				t.Fatalf("AttemptNumber = %d, want 2", chain.AttemptNumber)
			}
		},
	}
}

func rerunRootSecondAttemptCase() gateFailureCase {
	rerunRootID := domaintypes.NewJobID()
	heal1ID := domaintypes.NewJobID()
	failedReGateID := domaintypes.NewJobID()
	heal2ID := domaintypes.NewJobID()
	reGate2ID := domaintypes.NewJobID()
	return gateFailureCase{
		name: "rerun-root re-gate chain continues with second healing attempt",
		failedJob: store.Job{
			ID: failedReGateID, JobType: domaintypes.JobTypeReGate, RepoShaIn: testRepoSHAIn,
		},
		jobsByID: map[domaintypes.JobID]store.Job{
			rerunRootID:    {ID: rerunRootID, JobType: domaintypes.JobTypeReGate, NextID: &heal1ID},
			heal1ID:        {ID: heal1ID, JobType: domaintypes.JobTypeHeal, NextID: &failedReGateID},
			failedReGateID: {ID: failedReGateID, JobType: domaintypes.JobTypeReGate},
		},
		recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "code", StrategyID: "code-default"},
		heal:         basicHealSpec(3),
		newJobID:     newFixedIDSequence(heal2ID, reGate2ID),
		wantOutcome:  lifecycle.GateFailureOutcomeHealChain,
		assertChain: func(t *testing.T, chain *lifecycle.HealChainSpec) {
			t.Helper()
			if chain.AttemptNumber != 2 {
				t.Fatalf("AttemptNumber = %d, want 2", chain.AttemptNumber)
			}
		},
	}
}

func TestEvaluateGateFailureTransition(t *testing.T) {
	t.Parallel()

	cases := []gateFailureCase{
		{
			name:         "mixed recovery kind still attempts healing",
			failedJob:    store.Job{ID: domaintypes.NewJobID(), RepoShaIn: testRepoSHAIn},
			recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "mixed"},
			heal:         basicHealSpec(1),
			newJobID:     domaintypes.NewJobID,
			wantOutcome:  lifecycle.GateFailureOutcomeHealChain,
		},
		{
			name:         "no healing config cancels",
			failedJob:    store.Job{ID: domaintypes.NewJobID(), RepoShaIn: testRepoSHAIn},
			recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
			newJobID:     domaintypes.NewJobID,
			wantOutcome:  lifecycle.GateFailureOutcomeCancel,
		},
		{
			name:         "invalid SHA cancels",
			failedJob:    store.Job{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypePreGate, RepoShaIn: "not-a-valid-sha"},
			recoveryMeta: &contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
			heal:         basicHealSpec(1),
			newJobID:     domaintypes.NewJobID,
			wantOutcome:  lifecycle.GateFailureOutcomeCancel,
		},
		retriesExhaustedCase(),
		firstAttemptCase(),
		secondAttemptCase(),
		rerunRootSecondAttemptCase(),
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.jobsByID == nil {
				tc.jobsByID = map[domaintypes.JobID]store.Job{}
			}
			if tc.detectedStack == "" {
				tc.detectedStack = contracts.MigStackUnknown
			}

			decision, err := lifecycle.EvaluateGateFailureTransition(
				tc.failedJob, tc.jobsByID, tc.recoveryMeta,
				tc.detectedStack, tc.heal, tc.newJobID,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v", decision.Outcome, tc.wantOutcome)
			}
			if tc.wantOutcome == lifecycle.GateFailureOutcomeCancel && decision.Chain != nil {
				t.Fatal("expected no chain for cancel decision")
			}
			if tc.assertChain != nil {
				if decision.Chain == nil {
					t.Fatal("expected chain to be set")
				}
				tc.assertChain(t, decision.Chain)
			}
		})
	}
}

// ========== ResolveGateRecoveryContext ==========

func TestResolveGateRecoveryContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		job            store.Job
		wantErrorKind  string
		wantStack      contracts.MigStack
		wantStrategyID string
	}{
		{
			name:          "defaults when no meta",
			job:           store.Job{ID: domaintypes.NewJobID()},
			wantErrorKind: contracts.DefaultRecoveryErrorKind().String(),
			wantStack:     contracts.MigStackUnknown,
		},
		{
			name: "parses gate recovery",
			job: store.Job{
				ID:   domaintypes.NewJobID(),
				Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
			},
			wantErrorKind:  "infra",
			wantStack:      contracts.MigStackJavaMaven,
			wantStrategyID: "infra-default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta, stack, _ := lifecycle.ResolveGateRecoveryContext(tc.job)
			if meta.ErrorKind != tc.wantErrorKind {
				t.Fatalf("ErrorKind = %q, want %q", meta.ErrorKind, tc.wantErrorKind)
			}
			if stack != tc.wantStack {
				t.Fatalf("stack = %q, want %q", stack, tc.wantStack)
			}
			if tc.wantStrategyID != "" && meta.StrategyID != tc.wantStrategyID {
				t.Fatalf("StrategyID = %q, want %q", meta.StrategyID, tc.wantStrategyID)
			}
		})
	}
}
