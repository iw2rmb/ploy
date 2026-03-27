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
// These tests verify recovery-loop mechanics when jobs complete:
// - ReGate candidate promotion and metadata merging
// - Heal→ReGate candidate refresh (valid and missing)
// - PreGate generated gate profile promotion

func TestCompleteJob_ReGateSuccessPromotesValidatedCandidate(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
		resolveStackRowByLangToolResult: store.ResolveStackRowByLangToolRow{
			ID:      7,
			Lang:    "go",
			Tool:    "go",
			Release: "",
		},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.upsertExactGateProfileCalled {
		t.Fatal("expected UpsertExactGateProfile to be called")
	}
	if !st.upsertGateJobProfileLinkCalled {
		t.Fatal("expected UpsertGateJobProfileLink to be called")
	}
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called")
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
	if err != nil {
		t.Fatalf("unmarshal promoted meta: %v", err)
	}
	if meta.Recovery == nil || meta.Recovery.CandidatePromoted == nil || !*meta.Recovery.CandidatePromoted {
		t.Fatalf("candidate_promoted = %#v, want true", meta.Recovery)
	}
}

func TestCompleteJob_ReGateCompletionMergesExistingRecoveryMetadata(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}

	meta, err := contracts.UnmarshalJobMeta(st.updateJobCompletionWithMetaParams.Meta)
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

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Fail",
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.upsertExactGateProfileCalled {
		t.Fatal("did not expect gate profile persistence on failed re-gate")
	}
}

func TestCompleteJob_HealSuccessRefreshesNextReGateCandidate(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeHeal, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID
	f.Job.Meta = []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}`)

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID:          failedGateID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef,
		Attempt:     f.Job.Attempt,
		Name:        "pre-gate",
		Status:      domaintypes.JobStatusFail,
		JobType:     domaintypes.JobTypePreGate,
		NextID:      &f.Job.ID,
		Meta:        []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID:          reGateID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef,
		Attempt:     f.Job.Attempt,
		Name:        "re-gate-1",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeReGate,
		Meta:        []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
		getJobResults: map[domaintypes.JobID]store.Job{
			reGateID: reGate,
		},
		listJobsByRunRepoAttemptResult: []store.Job{failedGate, f.Job, reGate},
		listArtifactBundlesMetaByRunAndJobResult: []store.ArtifactBundle{
			{
				RunID:     f.RunID,
				JobID:     &f.Job.ID,
				ObjectKey: strPtr("artifacts/run/" + f.RunID.String() + "/bundle/heal.tar.gz"),
			},
		},
	}
	if _, stack, _ := lifecycle.ResolveGateRecoveryContext(failedGate); stack == contracts.ModStackUnknown {
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
	bundle := mustTarGzPayload(t, map[string][]byte{
		"out/gate-profile-candidate.json": candidateJSON,
	})
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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	if st.updateJobMetaParams.ID != reGateID {
		t.Fatalf("updated meta job_id = %s, want %s", st.updateJobMetaParams.ID, reGateID)
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
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

	f := newJobFixture(domaintypes.JobTypeHeal, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID:          failedGateID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef,
		Attempt:     f.Job.Attempt,
		Name:        "pre-gate",
		Status:      domaintypes.JobStatusFail,
		JobType:     domaintypes.JobTypePreGate,
		NextID:      &f.Job.ID,
		Meta:        []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID:          reGateID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef,
		Attempt:     f.Job.Attempt,
		Name:        "re-gate-1",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeReGate,
		Meta:        []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
		getJobResults: map[domaintypes.JobID]store.Job{
			reGateID: reGate,
		},
		listJobsByRunRepoAttemptResult: []store.Job{failedGate, f.Job, reGate},
	}
	bp := blobpersist.New(st, bsmock.New())

	handler := completeJobHandler(st, nil, bp)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
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
