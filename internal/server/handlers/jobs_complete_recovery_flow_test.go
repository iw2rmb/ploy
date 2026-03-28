package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// ===== Recovery Flow Tests =====

func TestCompleteJob_ReGateSuccessPromotesValidatedCandidate(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeReGate)
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := newMockStoreForJob(f,
		withResolveStackRow(store.ResolveStackRowByLangToolRow{
			ID: 7, Lang: "go", Tool: "go", Release: "",
		}),
	)

	handler := completeJobHandler(st, nil, nil, bsmock.New())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"static_checks": []any{map[string]any{"tool": "maven", "passed": true}},
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpsertExactGateProfile", st.upsertExactGateProfileCalled)
	assertCalled(t, "UpsertGateJobProfileLink", st.upsertGateJobProfileLink.called)
	assertCalled(t, "UpdateJobMeta", st.updateJobMeta.called)
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMeta.params.Meta)
	if err != nil {
		t.Fatalf("unmarshal promoted meta: %v", err)
	}
	if meta.Recovery == nil || meta.Recovery.CandidatePromoted == nil || !*meta.Recovery.CandidatePromoted {
		t.Fatalf("candidate_promoted = %#v, want true", meta.Recovery)
	}
}

func TestCompleteJob_ReGateCompletionMergesExistingRecoveryMetadata(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeReGate)
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := newMockStoreForJob(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"static_checks": []any{map[string]any{"tool": "maven", "passed": true}},
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpdateJobCompletionWithMeta", st.updateJobCompletionWithMeta.called)

	meta, err := contracts.UnmarshalJobMeta(st.updateJobCompletionWithMeta.params.Meta)
	if err != nil {
		t.Fatalf("unmarshal persisted meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected merged job-level recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if meta.Recovery.CandidatePromoted == nil || *meta.Recovery.CandidatePromoted {
		t.Fatalf("candidate_promoted = %#v, want false before promotion write", meta.Recovery.CandidatePromoted)
	}
}

func TestCompleteJob_ReGateFailureDoesNotPromoteCandidate(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeReGate)
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}}}`)

	st := newMockStoreForJob(f)

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.upsertExactGateProfileCalled {
		t.Fatal("did not expect gate profile persistence on failed re-gate")
	}
}

func TestCompleteJob_HealSuccessRefreshesNextReGateCandidate(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeHeal)
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID
	f.Job.Meta = []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}`)

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID: failedGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "pre-gate", Status: domaintypes.JobStatusFail,
		JobType: domaintypes.JobTypePreGate, NextID: &f.Job.ID,
		Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID: reGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "re-gate-1", Status: domaintypes.JobStatusCreated,
		JobType: domaintypes.JobTypeReGate,
		Meta:    []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := newMockStoreForJob(f,
		withJobResults(map[domaintypes.JobID]store.Job{reGateID: reGate}),
		withRepoAttemptJobs([]store.Job{failedGate, f.Job, reGate}),
		withArtifactBundles([]store.ArtifactBundle{
			{RunID: f.RunID, JobID: &f.Job.ID, ObjectKey: ptr("artifacts/run/" + f.RunID.String() + "/bundle/heal.tar.gz")},
		}),
	)
	if _, stack, _ := lifecycle.ResolveGateRecoveryContext(failedGate); stack == contracts.MigStackUnknown {
		t.Fatal("expected failed gate metadata to expose detected stack")
	}

	bs := bsmock.New()
	candidateJSON := []byte(`{
  "schema_version":1,
  "repo_id":"` + f.Job.RepoID.String() + `",
  "runner_mode":"simple",
  "stack":{"language":"java","tool":"maven"},
  "targets":{
    "active":"build",
    "build":{"status":"passed","command":"mvn test","env":{},"failure_code":null},
    "unit":{"status":"not_attempted","env":{}},
    "all_tests":{"status":"not_attempted","env":{}}
  },
  "orchestration":{"pre":[],"post":[]},
  "tactics_used":["unit_test_focused_profile"],
  "attempts":[],
  "evidence":{"log_refs":["/in/build-gate.log"],"diagnostics":[]},
  "repro_check":{"status":"failed","details":"not run"},
  "prompt_delta_suggestion":{"status":"none","summary":"","candidate_lines":[]}
}`)
	bundle := mustTarGzPayload(t, map[string][]byte{"out/gate-profile-candidate.json": candidateJSON})
	if _, err := bs.Put(context.Background(), "artifacts/run/"+f.RunID.String()+"/bundle/heal.tar.gz", "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(st, bs)

	handler := completeJobHandler(st, nil, bp)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobMeta.called {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	if st.updateJobMeta.params.ID != reGateID {
		t.Fatalf("updated meta job_id = %s, want %s", st.updateJobMeta.params.ID, reGateID)
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMeta.params.Meta)
	if err != nil {
		t.Fatalf("unmarshal updated re-gate meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q (error=%q)", got, want, meta.Recovery.CandidateValidationError)
	}
	if len(meta.Recovery.CandidateGateProfile) == 0 {
		t.Fatal("expected candidate_gate_profile payload")
	}
}

func TestCompleteJob_HealSuccessRefreshesNextReGateCandidateMissing(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeHeal)
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID: failedGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "pre-gate", Status: domaintypes.JobStatusFail,
		JobType: domaintypes.JobTypePreGate, NextID: &f.Job.ID,
		Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID: reGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "re-gate-1", Status: domaintypes.JobStatusCreated,
		JobType: domaintypes.JobTypeReGate,
		Meta:    []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := newMockStoreForJob(f,
		withJobResults(map[domaintypes.JobID]store.Job{reGateID: reGate}),
		withRepoAttemptJobs([]store.Job{failedGate, f.Job, reGate}),
	)
	bp := blobpersist.New(st, bsmock.New())

	handler := completeJobHandler(st, nil, bp)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobMeta.called {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMeta.params.Meta)
	if err != nil {
		t.Fatalf("unmarshal updated re-gate meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusMissing; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if meta.Recovery.CandidateValidationError == "" {
		t.Fatal("expected candidate_validation_error for missing artifact")
	}
}
