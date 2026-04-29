package runs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestOrderRepoJobsByChain_ReconstructsLinkedOrder(t *testing.T) {
	t.Parallel()

	pre := domaintypes.NewJobID()
	mig0 := domaintypes.NewJobID()
	mig1 := domaintypes.NewJobID()
	post := domaintypes.NewJobID()

	jobs := []RepoJobEntry{
		// Deliberately out of chain order (mirrors current broken render shape).
		{JobID: post, JobType: "post_gate", Status: domaintypes.JobStatusCreated, NextID: nil},
		{JobID: mig1, JobType: "mig", Status: domaintypes.JobStatusCreated, NextID: &post},
		{JobID: mig0, JobType: "mig", Status: domaintypes.JobStatusCreated, NextID: &mig1},
		{JobID: pre, JobType: "pre_gate", Status: domaintypes.JobStatusRunning, NextID: &mig0},
	}

	got := orderRepoJobsByChain(jobs)
	if len(got) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(got))
	}

	if got[0].JobID != pre || got[1].JobID != mig0 || got[2].JobID != mig1 || got[3].JobID != post {
		t.Fatalf("unexpected chain order: got [%s, %s, %s, %s]", got[0].JobID, got[1].JobID, got[2].JobID, got[3].JobID)
	}
}

func TestListRepoJobsCommand_DecodeJobContract(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s, want GET", r.Method)
		}
		wantPath := "/api/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/jobs"
		if r.URL.Path != wantPath {
			t.Fatalf("path=%s, want %s", r.URL.Path, wantPath)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"run_id":"` + runID.String() + `",
			"repo_id":"` + repoID.String() + `",
			"attempt":1,
			"jobs":[
					{
						"job_id":"` + jobID.String() + `",
					"name":"post-gate",
					"job_type":"post_gate",
					"job_image":"image:tag",
					"next_id":null,
					"node_id":null,
					"status":"Success",
					"duration_ms":123
				}
			]
		}`))
	}))
	t.Cleanup(srv.Close)

	baseURL, err := url.Parse(srv.URL + "/api")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	result, err := ListRepoJobsCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		RunID:   runID,
		RepoID:  repoID,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("list repo jobs: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs len=%d, want 1", len(result.Jobs))
	}
	if got, want := result.Jobs[0].JobType, domaintypes.JobTypePostGate; got != want {
		t.Fatalf("job_type=%q, want %q", got, want)
	}
	if got, want := result.Jobs[0].JobImage, "image:tag"; got != want {
		t.Fatalf("job_image=%q, want %q", got, want)
	}
}
