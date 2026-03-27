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
		name    string
		jobType domaintypes.JobType
		rrStatus domaintypes.RunRepoStatus
		wantAdvance bool
	}{
		{
			name:        "non-MR queued repo advances to running",
			jobType:     domaintypes.JobTypeMod,
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
			jobType:     domaintypes.JobTypeMod,
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
		{domaintypes.JobTypeMod, false},
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

func basicHealingSpec(retries int) *contracts.HealingSpec {
	return &contracts.HealingSpec{
		ByErrorKind: map[string]contracts.HealingActionSpec{
			"infra": {
				Retries: retries,
				Image:   contracts.JobImage{Universal: "heal:latest"},
			},
		},
	}
}

func TestEvaluateGateFailureTransition_TerminalRecoveryKindCancels(t *testing.T) {
	t.Parallel()

	failedJob := store.Job{
		ID:        domaintypes.NewJobID(),
		RepoShaIn: testRepoSHAIn,
	}

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		map[domaintypes.JobID]store.Job{},
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "mixed"},
		contracts.RecoveryErrorKindMixed,
		contracts.ModStackUnknown,
		basicHealingSpec(1),
		domaintypes.NewJobID,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeCancel {
		t.Fatalf("outcome = %v, want Cancel", decision.Outcome)
	}
	if decision.Chain != nil {
		t.Fatal("expected no chain for cancel decision")
	}
}

func TestEvaluateGateFailureTransition_NoHealingConfigCancels(t *testing.T) {
	t.Parallel()

	failedJob := store.Job{
		ID:        domaintypes.NewJobID(),
		RepoShaIn: testRepoSHAIn,
	}

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		map[domaintypes.JobID]store.Job{},
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		contracts.RecoveryErrorKindInfra,
		contracts.ModStackUnknown,
		nil,
		domaintypes.NewJobID,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeCancel {
		t.Fatalf("outcome = %v, want Cancel", decision.Outcome)
	}
}

func TestEvaluateGateFailureTransition_NoActionForErrorKindCancels(t *testing.T) {
	t.Parallel()

	failedJob := store.Job{
		ID:        domaintypes.NewJobID(),
		RepoShaIn: testRepoSHAIn,
	}

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		map[domaintypes.JobID]store.Job{},
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "deps"},
		contracts.RecoveryErrorKindDeps,
		contracts.ModStackUnknown,
		basicHealingSpec(1), // only has "infra" action
		domaintypes.NewJobID,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeCancel {
		t.Fatalf("outcome = %v, want Cancel", decision.Outcome)
	}
}

func TestEvaluateGateFailureTransition_RetriesExhaustedCancels(t *testing.T) {
	t.Parallel()

	baseGateID := domaintypes.NewJobID()
	heal1ID := domaintypes.NewJobID()
	reGate1ID := domaintypes.NewJobID()

	// Simulate: pre-gate → heal-1 → re-gate-1 (already failed)
	jobsByID := map[domaintypes.JobID]store.Job{
		baseGateID: {
			ID:      baseGateID,
			JobType: domaintypes.JobTypePreGate,
			NextID:  &heal1ID,
		},
		heal1ID: {
			ID:      heal1ID,
			JobType: domaintypes.JobTypeHeal,
			NextID:  &reGate1ID,
		},
		reGate1ID: {
			ID:      reGate1ID,
			JobType: domaintypes.JobTypeReGate,
		},
	}

	failedJob := store.Job{
		ID:        reGate1ID,
		JobType:   domaintypes.JobTypeReGate,
		RepoShaIn: testRepoSHAIn,
	}

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		jobsByID,
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		contracts.RecoveryErrorKindInfra,
		contracts.ModStackUnknown,
		basicHealingSpec(1), // retries=1, already have 1 attempt
		domaintypes.NewJobID,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeCancel {
		t.Fatalf("outcome = %v, want Cancel", decision.Outcome)
	}
}

func TestEvaluateGateFailureTransition_FirstAttemptCreatesChain(t *testing.T) {
	t.Parallel()

	baseGateID := domaintypes.NewJobID()
	successorID := domaintypes.NewJobID()
	failedJob := store.Job{
		ID:        baseGateID,
		JobType:   domaintypes.JobTypePreGate,
		RepoShaIn: testRepoSHAIn,
		NextID:    &successorID,
	}
	jobsByID := map[domaintypes.JobID]store.Job{
		baseGateID: failedJob,
	}

	healID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		jobsByID,
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "infra", StrategyID: "infra-default"},
		contracts.RecoveryErrorKindInfra,
		contracts.ModStackUnknown,
		basicHealingSpec(2),
		newFixedIDSequence(healID, reGateID),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeHealChain {
		t.Fatalf("outcome = %v, want HealChain", decision.Outcome)
	}

	chain := decision.Chain
	if chain == nil {
		t.Fatal("expected chain to be set")
	}
	if chain.ReGateID != reGateID {
		t.Fatalf("ReGateID = %s, want %s", chain.ReGateID, reGateID)
	}
	if chain.HealID != healID {
		t.Fatalf("HealID = %s, want %s", chain.HealID, healID)
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
	if chain.ReGateMeta == nil || chain.ReGateMeta.Recovery == nil {
		t.Fatal("expected ReGateMeta.Recovery to be set")
	}
	if chain.ReGateMeta.Recovery.ErrorKind != "infra" {
		t.Fatalf("ReGateMeta.Recovery.ErrorKind = %q, want %q", chain.ReGateMeta.Recovery.ErrorKind, "infra")
	}
	if chain.HealMeta == nil || chain.HealMeta.Recovery == nil {
		t.Fatal("expected HealMeta.Recovery to be set")
	}
}

func TestEvaluateGateFailureTransition_InvalidSHACancels(t *testing.T) {
	t.Parallel()

	failedJob := store.Job{
		ID:        domaintypes.NewJobID(),
		JobType:   domaintypes.JobTypePreGate,
		RepoShaIn: "not-a-valid-sha",
	}

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		map[domaintypes.JobID]store.Job{},
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		contracts.RecoveryErrorKindInfra,
		contracts.ModStackUnknown,
		basicHealingSpec(1),
		domaintypes.NewJobID,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeCancel {
		t.Fatalf("outcome = %v, want Cancel for invalid SHA", decision.Outcome)
	}
}

func TestEvaluateGateFailureTransition_SecondAttemptIncreasesNumber(t *testing.T) {
	t.Parallel()

	baseGateID := domaintypes.NewJobID()
	heal1ID := domaintypes.NewJobID()
	reGate1ID := domaintypes.NewJobID()
	successorID := domaintypes.NewJobID()

	// Simulate: pre-gate → heal-1 → re-gate-1 (already done once)
	jobsByID := map[domaintypes.JobID]store.Job{
		baseGateID: {
			ID:      baseGateID,
			JobType: domaintypes.JobTypePreGate,
			NextID:  &heal1ID,
		},
		heal1ID: {
			ID:      heal1ID,
			JobType: domaintypes.JobTypeHeal,
			NextID:  &reGate1ID,
		},
		reGate1ID: {
			ID:      reGate1ID,
			JobType: domaintypes.JobTypeReGate,
			NextID:  &successorID,
		},
		successorID: {
			ID:      successorID,
			JobType: domaintypes.JobTypeMod,
		},
	}

	failedJob := store.Job{
		ID:        reGate1ID,
		JobType:   domaintypes.JobTypeReGate,
		RepoShaIn: testRepoSHAIn,
		NextID:    &successorID,
	}

	heal2ID := domaintypes.NewJobID()
	reGate2ID := domaintypes.NewJobID()

	decision, err := lifecycle.EvaluateGateFailureTransition(
		failedJob,
		jobsByID,
		&contracts.BuildGateRecoveryMetadata{ErrorKind: "infra"},
		contracts.RecoveryErrorKindInfra,
		contracts.ModStackUnknown,
		basicHealingSpec(3), // retries=3, have 1 attempt
		newFixedIDSequence(heal2ID, reGate2ID),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Outcome != lifecycle.GateFailureOutcomeHealChain {
		t.Fatalf("outcome = %v, want HealChain", decision.Outcome)
	}
	if decision.Chain.AttemptNumber != 2 {
		t.Fatalf("AttemptNumber = %d, want 2", decision.Chain.AttemptNumber)
	}
}

// ========== ResolveGateRecoveryContext ==========

func TestResolveGateRecoveryContext_DefaultsWhenNoMeta(t *testing.T) {
	t.Parallel()

	job := store.Job{ID: domaintypes.NewJobID()}
	meta, stack, _ := lifecycle.ResolveGateRecoveryContext(job)

	if meta.ErrorKind != contracts.DefaultRecoveryErrorKind().String() {
		t.Fatalf("ErrorKind = %q, want default %q", meta.ErrorKind, contracts.DefaultRecoveryErrorKind())
	}
	if stack != contracts.ModStackUnknown {
		t.Fatalf("stack = %q, want unknown", stack)
	}
}

func TestResolveGateRecoveryContext_ParsesGateRecovery(t *testing.T) {
	t.Parallel()

	job := store.Job{
		ID:   domaintypes.NewJobID(),
		Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	meta, stack, _ := lifecycle.ResolveGateRecoveryContext(job)

	if meta.ErrorKind != "infra" {
		t.Fatalf("ErrorKind = %q, want infra", meta.ErrorKind)
	}
	if meta.StrategyID != "infra-default" {
		t.Fatalf("StrategyID = %q, want infra-default", meta.StrategyID)
	}
	if stack != contracts.ModStackJavaMaven {
		t.Fatalf("stack = %q, want java-maven", stack)
	}
}
