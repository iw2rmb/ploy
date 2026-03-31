package handlers

import (
	"context"
	"errors"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const healingTestRepoSHAIn = "0123456789abcdef0123456789abcdef01234567"

func TestMaybeCreateHealingJobs_FirstAttemptCreatesJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	hc := newHealingChain(t,
		withHealingMeta([]byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing"}}}`)),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", len(hc.Store.createJob.calls))
	}
	if hc.Store.createJob.calls[0].Name != "re-gate-1" {
		t.Fatalf("expected first created job to be re-gate-1 (tail-first FK-safe insert), got %q", hc.Store.createJob.calls[0].Name)
	}
	if hc.Store.createJob.calls[1].Name != "heal-1-0" {
		t.Fatalf("expected second created job to be heal-1-0, got %q", hc.Store.createJob.calls[1].Name)
	}

	byName := createJobsByName(hc.Store.createJob.calls)

	healJob := byName["heal-1-0"]
	if healJob.Status != domaintypes.JobStatusQueued {
		t.Fatalf("expected heal-1-0 status=Queued, got %s", healJob.Status)
	}
	if healJob.JobImage != "codex:latest" {
		t.Fatalf("expected heal-1-0 image=codex:latest, got %q", healJob.JobImage)
	}
	if healJob.NextID == nil {
		t.Fatalf("expected heal-1-0 next_id to be set")
	}
	if healJob.RepoShaIn != healingTestRepoSHAIn {
		t.Fatalf("expected heal-1-0 repo_sha_in=%q, got %q", healingTestRepoSHAIn, healJob.RepoShaIn)
	}

	reGateJob := byName["re-gate-1"]
	if reGateJob.Status != domaintypes.JobStatusCreated {
		t.Fatalf("expected re-gate-1 status=Created, got %s", reGateJob.Status)
	}
	if healJob.NextID == nil || *healJob.NextID != reGateJob.ID {
		t.Fatalf("expected heal to point to re-gate")
	}
	if reGateJob.NextID == nil || *reGateJob.NextID != hc.SuccessorID {
		t.Fatalf("expected re-gate to preserve original successor %s", hc.SuccessorID)
	}
	if len(hc.Store.updateJobNextIDParams) != 1 {
		t.Fatalf("expected one next_id rewiring update, got %d", len(hc.Store.updateJobNextIDParams))
	}
	if hc.Store.updateJobNextIDParams[0].ID != hc.FailedJob.ID {
		t.Fatalf("expected failed job %s to be rewired, got %s", hc.FailedJob.ID, hc.Store.updateJobNextIDParams[0].ID)
	}
	if hc.Store.updateJobNextIDParams[0].NextID == nil || *hc.Store.updateJobNextIDParams[0].NextID != healJob.ID {
		t.Fatalf("expected failed job to point to heal job %s", healJob.ID)
	}

	reGateMeta, err := contracts.UnmarshalJobMeta(reGateJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if reGateMeta.RecoveryMetadata == nil || reGateMeta.RecoveryMetadata.ErrorKind != "infra" {
		t.Fatalf("expected re-gate recovery.error_kind=infra, got %#v", reGateMeta.RecoveryMetadata)
	}
	if got, want := reGateMeta.RecoveryMetadata.StrategyID, "infra-default"; got != want {
		t.Fatalf("re-gate recovery.strategy_id = %q, want %q", got, want)
	}

	healMeta, err := contracts.UnmarshalJobMeta(healJob.Meta)
	if err != nil {
		t.Fatalf("unmarshal heal meta: %v", err)
	}
	if healMeta.RecoveryMetadata == nil || healMeta.RecoveryMetadata.ErrorKind != "infra" {
		t.Fatalf("expected heal recovery.error_kind=infra, got %#v", healMeta.RecoveryMetadata)
	}
}

func TestMaybeCreateHealingJobs_SecondAttemptUsesExistingHealJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	gateMeta := []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`)
	hc := newHealingChain(t,
		withHealingMeta(gateMeta),
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 3) }),
		withPriorHeals(
			priorHealJob{Name: "heal-1-0", JobType: domaintypes.JobTypeHeal, Status: domaintypes.JobStatusSuccess, Meta: []byte(`{}`)},
			priorHealJob{Name: "re-gate-1", JobType: domaintypes.JobTypeReGate, Status: domaintypes.JobStatusFail, ShaIn: healingTestRepoSHAIn, Meta: gateMeta},
		),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	if len(hc.Store.createJob.calls) != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", len(hc.Store.createJob.calls))
	}
	if hc.Store.createJob.calls[0].Name != "re-gate-2" {
		t.Fatalf("expected first healing job name re-gate-2, got %q", hc.Store.createJob.calls[0].Name)
	}
	if hc.Store.createJob.calls[0].JobType != domaintypes.JobTypeReGate {
		t.Fatalf("expected first job JobType=re_gate, got %q", hc.Store.createJob.calls[0].JobType)
	}
	if hc.Store.createJob.calls[1].Name != "heal-2-0" {
		t.Fatalf("expected second healing job name heal-2-0, got %q", hc.Store.createJob.calls[1].Name)
	}
	if hc.Store.createJob.calls[1].JobType != domaintypes.JobTypeHeal {
		t.Fatalf("expected second job JobType=heal, got %q", hc.Store.createJob.calls[1].JobType)
	}
	if hc.Store.createJob.calls[0].NextID == nil || *hc.Store.createJob.calls[0].NextID != hc.SuccessorID {
		t.Fatalf("expected re-gate-2 to link back to original successor %s", hc.SuccessorID)
	}
}

// TestMaybeCreateHealingJobs_CancelsRemaining covers cases where healing cannot proceed
// and the successor must be cancelled instead.
func TestMaybeCreateHealingJobs_CancelsRemaining(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		errorKind   string
		repoShaIn   string
		specSetup   func(st *jobStore)
		extraAssert func(t *testing.T, st *jobStore)
	}{
		{
			name:      "mixed_classification",
			errorKind: "mixed",
			repoShaIn: healingTestRepoSHAIn,
		},
		{
			name:      "invalid_repo_sha_in",
			errorKind: "infra",
			repoShaIn: "invalid",
		},
		{
			name:      "terminal_without_spec_fetch",
			errorKind: "mixed",
			repoShaIn: healingTestRepoSHAIn,
			specSetup: func(st *jobStore) {
				st.getSpec.err = errors.New("db unavailable")
			},
			extraAssert: func(t *testing.T, st *jobStore) {
				t.Helper()
				assertNotCalled(t, "GetSpec", st.getSpec.called)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			hc := newHealingChain(t,
				withHealingMeta([]byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"`+tc.errorKind+`","strategy_id":"`+tc.errorKind+`-default"}}}`)),
				withHealingRepoShaIn(tc.repoShaIn),
			)
			if tc.specSetup != nil {
				tc.specSetup(hc.Store)
			}

			if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
				t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
			}

			assertCancelsSuccessor(t, hc.Store, hc.SuccessorID)

			if tc.extraAssert != nil {
				tc.extraAssert(t, hc.Store)
			}
		})
	}
}

func TestMaybeCreateHealingJobs_ReGateInfraCandidateValidatedFromPreviousHeal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	reGateMeta := []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}}`)
	hc := newHealingChain(t,
		withHealingMeta(nil), // pre-gate has no meta in this scenario
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 3, withArtifactExpectations()) }),
		withPriorHeals(
			priorHealJob{Name: "heal-1-0", JobType: domaintypes.JobTypeHeal, Status: domaintypes.JobStatusSuccess, Meta: []byte(`{"kind":"mig"}`)},
			priorHealJob{Name: "re-gate-1", JobType: domaintypes.JobTypeReGate, Status: domaintypes.JobStatusFail, ShaIn: healingTestRepoSHAIn, Meta: reGateMeta},
		),
	)

	// Set up blob store with candidate artifact from the prior heal job.
	heal1ID := hc.Jobs[1].ID // heal-1-0
	objKey := "artifacts/run/" + hc.RunID.String() + "/bundle/heal-1.tar.gz"
	hc.Store.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{RunID: hc.RunID, JobID: &heal1ID, ObjectKey: ptr(objKey)},
	}

	candidateJSON := []byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"maven"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"go test ./...","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []},
		"tactics_used": ["go_default"],
		"attempts": [],
		"evidence": {"log_refs": ["inline://prep/test"], "diagnostics": []},
		"repro_check": {"status":"passed","details":"ok"},
		"prompt_delta_suggestion": {"status":"none","summary":"","candidate_lines":[]}
	}`)
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/gate-profile-candidate.json": candidateJSON,
	})
	if _, err := bs.Put(ctx, objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(hc.Store, bs)

	if err := maybeCreateHealingJobs(ctx, hc.Store, bp, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}
	if len(hc.Store.createJob.calls) != 2 {
		t.Fatalf("expected 2 CreateJob calls, got %d", len(hc.Store.createJob.calls))
	}

	createdReGateMeta, err := contracts.UnmarshalJobMeta(hc.Store.createJob.calls[0].Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if createdReGateMeta.RecoveryMetadata == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateSchemaID, contracts.GateProfileCandidateSchemaID; got != want {
		t.Fatalf("candidate_schema_id = %q, want %q", got, want)
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateArtifactPath, contracts.GateProfileCandidateArtifactPath; got != want {
		t.Fatalf("candidate_artifact_path = %q, want %q", got, want)
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q (error=%q)", got, want, createdReGateMeta.RecoveryMetadata.CandidateValidationError)
	}
	if len(createdReGateMeta.RecoveryMetadata.CandidateGateProfile) == 0 {
		t.Fatal("expected candidate_gate_profile to be stored")
	}
}

func TestMaybeCreateHealingJobs_FirstInsertionInfraCandidateMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	hc := newHealingChain(t,
		withHealingSpec(func(t *testing.T) []byte { return buildHealingSpec(t, 2, withArtifactExpectations()) }),
	)

	if err := maybeCreateHealingJobs(ctx, hc.Store, nil, hc.Run, hc.FailedJob); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	createdReGateMeta, err := contracts.UnmarshalJobMeta(hc.Store.createJob.calls[0].Meta)
	if err != nil {
		t.Fatalf("unmarshal re-gate meta: %v", err)
	}
	if createdReGateMeta.RecoveryMetadata == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := createdReGateMeta.RecoveryMetadata.CandidateValidationStatus, contracts.RecoveryCandidateStatusMissing; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if createdReGateMeta.RecoveryMetadata.CandidateValidationError == "" {
		t.Fatal("expected candidate_validation_error for missing candidate")
	}
}

func TestMaybeCompleteMultiStepRun_FinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runID := domaintypes.NewRunID()

	st := &jobStore{}
	st.countRunReposByStatus.val = []store.CountRunReposByStatusRow{
		{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		{Status: domaintypes.RunRepoStatusFail, Count: 1},
		}

	run := store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	if _, err := recovery.MaybeCompleteRunIfAllReposTerminal(ctx, st, nil, run); err != nil {
		t.Fatalf("maybeCompleteRunIfAllReposTerminal returned error: %v", err)
	}

	if !st.updateRunStatus.called {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatus.params.ID != runID || st.updateRunStatus.params.Status != domaintypes.RunStatusFinished {
		t.Fatalf("unexpected UpdateRunStatus params: %+v", st.updateRunStatus.params)
	}
}

func TestLoadRecoveryArtifact_Success(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "artifacts/run/" + runID.String() + "/bundle/test.tar.gz"

	st := &jobStore{}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{
		{
			RunID:     runID,
			JobID:     &jobID,
			ObjectKey: ptr(objKey),
		},
		}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/gate-profile-candidate.json": []byte(`{"schema_version":1}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}

	bp := blobpersist.New(st, bs)
	raw, err := loadRecoveryArtifact(context.Background(), bp, runID, jobID, "/out/gate-profile-candidate.json")
	if err != nil {
		t.Fatalf("loadRecoveryArtifact error: %v", err)
	}
	if string(raw) != `{"schema_version":1}` {
		t.Fatalf("unexpected payload: %s", string(raw))
	}
}

func TestLoadRecoveryArtifact_TypedErrors(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	bp := blobpersist.New(st, bsmock.New())

	_, err := loadRecoveryArtifact(context.Background(), bp, runID, jobID, "/out/gate-profile-candidate.json")
	if !errors.Is(err, blobpersist.ErrRecoveryArtifactNotFound) {
		t.Fatalf("expected ErrRecoveryArtifactNotFound, got %v", err)
	}
}

func TestCandidateMatchesDetectedStack_ReleaseAware(t *testing.T) {
	t.Parallel()

	profile, err := contracts.ParseGateProfileJSON([]byte(`{
		"schema_version": 1,
		"repo_id": "repo_123",
		"runner_mode": "simple",
		"stack": {"language":"java","tool":"gradle","release":"11"},
		"targets": {
			"active": "build",
			"build": {"status":"passed","command":"./gradlew test","env":{},"failure_code":null},
			"unit": {"status":"not_attempted","env":{}},
			"all_tests": {"status":"not_attempted","env":{}}
		},
		"orchestration": {"pre": [], "post": []},
		"tactics_used": [],
		"attempts": [],
		"evidence": {"log_refs": [], "diagnostics": []},
		"repro_check": {"status": "failed", "details": ""},
		"prompt_delta_suggestion": {"status":"none","summary":"","candidate_lines":[]}
	}`))
	if err != nil {
		t.Fatalf("ParseGateProfileJSON: %v", err)
	}

	if !candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "11",
	}) {
		t.Fatal("expected exact release match to pass")
	}
	if candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "17",
	}) {
		t.Fatal("expected mismatched release to fail")
	}
	if !candidateMatchesDetectedStack(profile, &contracts.StackExpectation{
		Language: "java",
		Tool:     "gradle",
		Release:  "",
	}) {
		t.Fatal("expected empty detected release to act as wildcard")
	}
}
