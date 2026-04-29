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

func TestListRunRepoJobsHandler_ExposesGateAndMigJobTypes(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	preGateID := domaintypes.NewJobID()
	migID := domaintypes.NewJobID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      preGateID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			Name:    "pre-gate",
			JobType: domaintypes.JobTypePreGate,
			Status:  domaintypes.JobStatusSuccess,
			Meta:    []byte(`{"kind":"gate"}`),
		},
		{
			ID:      migID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			Name:    "mig-0",
			JobType: domaintypes.JobTypeMig,
			Status:  domaintypes.JobStatusSuccess,
			Meta:    []byte(`{"kind":"mig"}`),
		},
	}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())
	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %T len=%d", resp["jobs"], len(jobs))
	}
	seen := map[string]bool{}
	for _, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("job payload type = %T, want object", raw)
		}
		if jt, ok := job["job_type"].(string); ok {
			seen[jt] = true
		}
	}
	if !seen["pre_gate"] {
		t.Fatalf("expected job_type %q in response, got %+v", "pre_gate", seen)
	}
	if !seen["mig"] {
		t.Fatalf("expected job_type %q in response, got %+v", "mig", seen)
	}
}

func TestListRunRepoJobsHandler_ExposesSBOMEvidence(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	preGateID := domaintypes.NewJobID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      preGateID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			Name:    "pre-gate",
			JobType: domaintypes.JobTypePreGate,
			Status:  domaintypes.JobStatusSuccess,
			Meta:    []byte(`{"kind":"gate"}`),
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{{}}
	st.listSBOMRowsByJob.val = []store.Sbom{
		{JobID: preGateID, RepoID: repoID, Lib: "org.slf4j:slf4j-api", Ver: "1.7.36"},
		{JobID: preGateID, RepoID: repoID, Lib: "junit:junit", Ver: "4.13.2"},
	}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())
	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			Name         string `json:"name"`
			JobType      string `json:"job_type"`
			SBOMEvidence *struct {
				ArtifactPresent    *bool `json:"artifact_present"`
				ParsedPackageCount *int  `json:"parsed_package_count"`
			} `json:"sbom_evidence"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Jobs))
	}
	gateJob := resp.Jobs[0]
	if got, want := gateJob.JobType, "pre_gate"; got != want {
		t.Fatalf("job_type = %q, want %q", got, want)
	}
	if gateJob.SBOMEvidence == nil {
		t.Fatal("expected sbom_evidence for gate job")
	}
	if gateJob.SBOMEvidence.ArtifactPresent == nil || !*gateJob.SBOMEvidence.ArtifactPresent {
		t.Fatalf("sbom_evidence.artifact_present = %#v, want true", gateJob.SBOMEvidence.ArtifactPresent)
	}
	if gateJob.SBOMEvidence.ParsedPackageCount == nil || *gateJob.SBOMEvidence.ParsedPackageCount != 2 {
		t.Fatalf("sbom_evidence.parsed_package_count = %#v, want 2", gateJob.SBOMEvidence.ParsedPackageCount)
	}
}

func TestListRunRepoJobsHandler_ExposesSBOMEvidenceWithNoArtifacts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	preGateID := domaintypes.NewJobID()

	st := &runStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{
			ID:      preGateID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
			Name:    "pre-gate",
			JobType: domaintypes.JobTypePreGate,
			Status:  domaintypes.JobStatusSuccess,
			Meta:    []byte(`{"kind":"gate"}`),
		},
	}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{}
	st.listSBOMRowsByJob.val = []store.Sbom{}

	handler := listRunRepoJobsHandler(st)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())
	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			SBOMEvidence *struct {
				ArtifactPresent    *bool `json:"artifact_present"`
				ParsedPackageCount *int  `json:"parsed_package_count"`
			} `json:"sbom_evidence"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp.Jobs))
	}
	if resp.Jobs[0].SBOMEvidence == nil {
		t.Fatal("expected sbom_evidence")
	}
	if resp.Jobs[0].SBOMEvidence.ArtifactPresent == nil || *resp.Jobs[0].SBOMEvidence.ArtifactPresent {
		t.Fatalf("artifact_present = %#v, want false", resp.Jobs[0].SBOMEvidence.ArtifactPresent)
	}
	if resp.Jobs[0].SBOMEvidence.ParsedPackageCount == nil || *resp.Jobs[0].SBOMEvidence.ParsedPackageCount != 0 {
		t.Fatalf("parsed_package_count = %#v, want 0", resp.Jobs[0].SBOMEvidence.ParsedPackageCount)
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

	metaJSON := `{"kind":"gate","gate":{"bug_summary":"missing ; in Foo.java"}}`
	_, handler, runID, repoID := newRunRepoJobsFixture(t, metaJSON)
	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil, "run_id", runID.String(), "repo_id", repoID.String())

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Jobs []struct {
			BugSummary string `json:"bug_summary"`
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
