package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunRepoJobsHandler_NextIDContract(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:         jobID,
			RunID:      runID,
			RepoID:     repoID,
			Attempt:    1,
			Name:       "mig-0",
			JobType:    "mig",
			JobImage:   "docker.io/example/mig:latest",
			RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
			RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
			NextID:     &nextID,
			Status:     domaintypes.JobStatusQueued,
			Meta:       []byte(`{"kind":"mig","mig_step_name":"hello"}`),
		},
		}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)
	if !st.listJobsByRunRepoAttempt.called {
		t.Fatal("expected ListJobsByRunRepoAttempt to be called")
	}

	resp := decodeBody[map[string]any](t, rr)

	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected one job entry, got %T len=%d", resp["jobs"], len(jobs))
	}
	job, ok := jobs[0].(map[string]any)
	if !ok {
		t.Fatalf("job payload type = %T, want object", jobs[0])
	}
	if got := job["job_type"]; got != "mig" {
		t.Fatalf("job_type = %v, want %q", got, "mig")
	}
	if got := job["job_image"]; got != "docker.io/example/mig:latest" {
		t.Fatalf("job_image = %v, want %q", got, "docker.io/example/mig:latest")
	}
	if got := job["repo_sha_in"]; got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("repo_sha_in = %v, want %q", got, "0123456789abcdef0123456789abcdef01234567")
	}
	if got := job["repo_sha_out"]; got != "89abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("repo_sha_out = %v, want %q", got, "89abcdef0123456789abcdef0123456789abcdef")
	}
	if got := job["next_id"]; got != nextID.String() {
		t.Fatalf("next_id = %v, want %q", got, nextID.String())
	}
}

func TestListRunRepoJobsHandler_AttemptQueryOverride(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs?attempt=3", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)
	if got := st.listJobsByRunRepoAttempt.params.Attempt; got != 3 {
		t.Fatalf("query attempt override not applied: got %d want %d", got, 3)
	}
}

func TestListRunRepoJobsHandler_OrdersJobsByChain(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	pre := domaintypes.NewJobID()
	mig0 := domaintypes.NewJobID()
	mig1 := domaintypes.NewJobID()
	post := domaintypes.NewJobID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: post, RunID: runID, RepoID: repoID, Attempt: 1, Name: "post-gate", JobType: "post_gate", Status: domaintypes.JobStatusCreated},
		{ID: mig1, RunID: runID, RepoID: repoID, Attempt: 1, Name: "mig-1", JobType: "mig", NextID: &post, Status: domaintypes.JobStatusCreated},
		{ID: mig0, RunID: runID, RepoID: repoID, Attempt: 1, Name: "mig-0", JobType: "mig", NextID: &mig1, Status: domaintypes.JobStatusCreated},
		{ID: pre, RunID: runID, RepoID: repoID, Attempt: 1, Name: "pre-gate", JobType: "pre_gate", NextID: &mig0, Status: domaintypes.JobStatusQueued},
		}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)

	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 4 {
		t.Fatalf("expected four job entries, got %T len=%d", resp["jobs"], len(jobs))
	}

	gotJobIDs := make([]string, 0, len(jobs))
	for _, raw := range jobs {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("job payload type = %T, want object", raw)
		}
		id, ok := entry["job_id"].(string)
		if !ok {
			t.Fatalf("job_id type = %T, want string", entry["job_id"])
		}
		gotJobIDs = append(gotJobIDs, id)
	}

	wantJobIDs := []string{pre.String(), mig0.String(), mig1.String(), post.String()}
	for i := range wantJobIDs {
		if gotJobIDs[i] != wantJobIDs[i] {
			t.Fatalf("job order mismatch at index %d: got %q want %q (full=%v)", i, gotJobIDs[i], wantJobIDs[i], gotJobIDs)
		}
	}
}

func TestListRunRepoJobsHandler_ExposesGateBugSummary(t *testing.T) {
	t.Parallel()

	metaJSON := `{"kind":"gate","gate":{"bug_summary":"missing ; in Foo.java","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}}`
	_, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			BugSummary string `json:"bug_summary"`
			Recovery   *struct {
				LoopKind     string   `json:"loop_kind"`
				StrategyID   string   `json:"strategy_id"`
				Confidence   *float64 `json:"confidence"`
				Reason       string   `json:"reason"`
				Expectations struct {
					Artifacts []struct {
						Path   string `json:"path"`
						Schema string `json:"schema"`
					} `json:"artifacts"`
				} `json:"expectations"`
			} `json:"recovery"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %d", len(resp.Jobs))
	}
	if got, want := resp.Jobs[0].BugSummary, "missing ; in Foo.java"; got != want {
		t.Fatalf("bug_summary = %q, want %q", got, want)
	}
	if resp.Jobs[0].Recovery == nil {
		t.Fatal("expected recovery field to be projected")
	}
	if got, want := resp.Jobs[0].Recovery.LoopKind, "healing"; got != want {
		t.Fatalf("recovery.loop_kind = %q, want %q", got, want)
	}
	if got, want := resp.Jobs[0].Recovery.StrategyID, "infra-default"; got != want {
		t.Fatalf("recovery.strategy_id = %q, want %q", got, want)
	}
	if resp.Jobs[0].Recovery.Confidence == nil || *resp.Jobs[0].Recovery.Confidence != 0.8 {
		t.Fatalf("recovery.confidence = %#v, want %v", resp.Jobs[0].Recovery.Confidence, 0.8)
	}
	if got, want := resp.Jobs[0].Recovery.Reason, "docker socket missing"; got != want {
		t.Fatalf("recovery.reason = %q, want %q", got, want)
	}
	if len(resp.Jobs[0].Recovery.Expectations.Artifacts) != 1 {
		t.Fatalf("recovery.expectations.artifacts len = %d, want 1", len(resp.Jobs[0].Recovery.Expectations.Artifacts))
	}
	if got, want := resp.Jobs[0].Recovery.Expectations.Artifacts[0].Path, "/out/gate-profile-candidate.json"; got != want {
		t.Fatalf("recovery.expectations.artifacts[0].path = %q, want %q", got, want)
	}
}

func TestListRunRepoJobsHandler_ExposesJobLevelRecovery(t *testing.T) {
	t.Parallel()

	metaJSON := `{"kind":"mig","action_summary":"updated deps","recovery":{"loop_kind":"healing","error_kind":"code","strategy_id":"code-default","reason":"compile failure"}}`
	st, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	// Override job type/status for heal job.
	st.listJobsByRunRepoAttempt.val[0].Name = "heal"
	st.listJobsByRunRepoAttempt.val[0].JobType = "heal"
	st.listJobsByRunRepoAttempt.val[0].Status = domaintypes.JobStatusSuccess

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			Recovery *struct {
				LoopKind  string `json:"loop_kind"`
			} `json:"recovery"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %d", len(resp.Jobs))
	}
	if resp.Jobs[0].Recovery == nil {
		t.Fatal("expected recovery field")
	}
	if got, want := resp.Jobs[0].Recovery.LoopKind, "healing"; got != want {
		t.Fatalf("recovery.loop_kind = %q, want %q", got, want)
	}
}

func TestListRunRepoJobsHandler_ExposesRecoveryCandidateAuditFields(t *testing.T) {
	t.Parallel()

	metaJSON := `{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"invalid","candidate_validation_error":"schema mismatch","candidate_promoted":false}}`
	st, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	// Override job type/status for re-gate job.
	st.listJobsByRunRepoAttempt.val[0].Name = "re-gate-1"
	st.listJobsByRunRepoAttempt.val[0].JobType = "re_gate"
	st.listJobsByRunRepoAttempt.val[0].Status = domaintypes.JobStatusSuccess

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			Recovery *struct {
				CandidateSchemaID         string `json:"candidate_schema_id"`
				CandidateArtifactPath     string `json:"candidate_artifact_path"`
				CandidateValidationStatus string `json:"candidate_validation_status"`
				CandidateValidationError  string `json:"candidate_validation_error"`
				CandidatePromoted         *bool  `json:"candidate_promoted"`
			} `json:"recovery"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %d", len(resp.Jobs))
	}
	if resp.Jobs[0].Recovery == nil {
		t.Fatal("expected recovery field")
	}
	if got, want := resp.Jobs[0].Recovery.CandidateSchemaID, "gate_profile_v1"; got != want {
		t.Fatalf("candidate_schema_id = %q, want %q", got, want)
	}
	if got, want := resp.Jobs[0].Recovery.CandidateArtifactPath, "/out/gate-profile-candidate.json"; got != want {
		t.Fatalf("candidate_artifact_path = %q, want %q", got, want)
	}
	if got, want := resp.Jobs[0].Recovery.CandidateValidationStatus, "invalid"; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if got := resp.Jobs[0].Recovery.CandidatePromoted; got == nil || *got {
		t.Fatalf("candidate_promoted = %#v, want false", got)
	}
}

func TestListRunRepoJobsHandler_ExposesGateStackDetection(t *testing.T) {
	t.Parallel()

	metaJSON := `{"kind":"gate","gate":{"detected_stack":{"language":"java","tool":"maven","release":"17"},"bug_summary":"build failed"}}`
	_, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			Lang    string `json:"lang"`
			Tooling string `json:"tooling"`
			Version string `json:"version"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %d", len(resp.Jobs))
	}
	if got, want := resp.Jobs[0].Lang, "java"; got != want {
		t.Fatalf("lang = %q, want %q", got, want)
	}
	if got, want := resp.Jobs[0].Tooling, "maven"; got != want {
		t.Fatalf("tooling = %q, want %q", got, want)
	}
	if got, want := resp.Jobs[0].Version, "17"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func TestListRunRepoJobsHandler_PrefersGateRuntimeImageAddress(t *testing.T) {
	t.Parallel()

	metaJSON := `{"kind":"gate","gate":{"stack_gate":{"enabled":true,"result":"pass","runtime_image":"ghcr.io/iw2rmb/ploy/gates/maven:3-eclipse-temurin-17"}}}`
	st, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	st.listJobsByRunRepoAttempt.val[0].JobImage = "gate-image-placeholder"

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			JobImage string `json:"job_image"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %d", len(resp.Jobs))
	}
	if got, want := resp.Jobs[0].JobImage, "ghcr.io/iw2rmb/ploy/gates/maven:3-eclipse-temurin-17"; got != want {
		t.Fatalf("job_image = %q, want %q", got, want)
	}
}

func TestListRunRepoJobsHandler_InvalidMeta_DoesNotFailResponse(t *testing.T) {
	t.Parallel()

	_, handler, runID, repoID := newRunRepoJobsFixture(t, `{"gate":{"bug_summary":"missing kind"}}`)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)
}
