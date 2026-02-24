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
	repoID := domaintypes.NewModRepoID()
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
				Name:     "mod-0",
				JobType:  "mod",
				JobImage: "docker.io/example/mod:latest",
				NextID:   &nextID,
				Status:   store.JobStatusQueued,
				Meta:     []byte(`{"kind":"mod","mods_step_name":"hello"}`),
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
	if got := job["job_type"]; got != "mod" {
		t.Fatalf("job_type = %v, want %q", got, "mod")
	}
	if got := job["job_image"]; got != "docker.io/example/mod:latest" {
		t.Fatalf("job_image = %v, want %q", got, "docker.io/example/mod:latest")
	}
	if got := job["next_id"]; got != nextID.String() {
		t.Fatalf("next_id = %v, want %q", got, nextID.String())
	}
}

func TestListRunRepoJobsHandler_AttemptQueryOverride(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewModRepoID()

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
