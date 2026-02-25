package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunRepoJobsHandler_NextIDContract(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()

	st := &mockStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:       jobID,
				RunID:    runID,
				RepoID:   repoID,
				Attempt:  1,
				Name:     "mig-0",
				JobType:  "mig",
				JobImage: "docker.io/example/mig:latest",
				NextID:   &nextID,
				Status:   store.JobStatusQueued,
				Meta:     []byte(`{"kind":"mig","mods_step_name":"hello"}`),
			},
		},
	}

	handler := listRunRepoJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.listJobsByRunRepoAttemptCalled {
		t.Fatal("expected ListJobsByRunRepoAttempt to be called")
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

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
	if got := job["next_id"]; got != nextID.String() {
		t.Fatalf("next_id = %v, want %q", got, nextID.String())
	}
}

func TestListRunRepoJobsHandler_AttemptQueryOverride(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

	st := &mockStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{},
	}

	handler := listRunRepoJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs?attempt=3", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := st.listJobsByRunRepoAttemptParams.Attempt; got != 3 {
		t.Fatalf("query attempt override not applied: got %d want %d", got, 3)
	}
}

func TestListRunRepoJobsHandler_OrdersJobsByChain(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	pre := domaintypes.NewJobID()
	mig0 := domaintypes.NewJobID()
	mig1 := domaintypes.NewJobID()
	post := domaintypes.NewJobID()

	st := &mockStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{ID: post, RunID: runID, RepoID: repoID, Attempt: 1, Name: "post-gate", JobType: "post_gate", Status: store.JobStatusCreated},
			{ID: mig1, RunID: runID, RepoID: repoID, Attempt: 1, Name: "mig-1", JobType: "mig", NextID: &post, Status: store.JobStatusCreated},
			{ID: mig0, RunID: runID, RepoID: repoID, Attempt: 1, Name: "mig-0", JobType: "mig", NextID: &mig1, Status: store.JobStatusCreated},
			{ID: pre, RunID: runID, RepoID: repoID, Attempt: 1, Name: "pre-gate", JobType: "pre_gate", NextID: &mig0, Status: store.JobStatusQueued},
		},
	}

	handler := listRunRepoJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

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
