package runs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestOrderRepoJobsByChain_ReconstructsLinkedOrder(t *testing.T) {
	t.Parallel()

	pre := domaintypes.NewJobID()
	mig0 := domaintypes.NewJobID()
	mig1 := domaintypes.NewJobID()
	post := domaintypes.NewJobID()

	jobs := []RepoJobEntry{
		// Deliberately out of chain order (mirrors current broken render shape).
		{JobID: post, JobType: "post_gate", Status: store.JobStatusCreated, NextID: nil},
		{JobID: mig1, JobType: "mig", Status: store.JobStatusCreated, NextID: &post},
		{JobID: mig0, JobType: "mig", Status: store.JobStatusCreated, NextID: &mig1},
		{JobID: pre, JobType: "pre_gate", Status: store.JobStatusRunning, NextID: &mig0},
	}

	got := orderRepoJobsByChain(jobs)
	if len(got) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(got))
	}

	if got[0].JobID != pre || got[1].JobID != mig0 || got[2].JobID != mig1 || got[3].JobID != post {
		t.Fatalf("unexpected chain order: got [%s, %s, %s, %s]", got[0].JobID, got[1].JobID, got[2].JobID, got[3].JobID)
	}
}

func TestListRepoJobsCommand_DecodeRecoveryContract(t *testing.T) {
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
					"name":"re-gate-1",
					"job_type":"re_gate",
					"job_image":"image:tag",
					"next_id":null,
					"node_id":null,
					"status":"Success",
					"duration_ms":123,
					"recovery":{
						"loop_kind":"healing",
						"error_kind":"infra",
						"strategy_id":"infra-default",
						"confidence":0.8,
						"reason":"docker socket missing",
						"expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]},
						"candidate_schema_id":"gate_profile_v1",
						"candidate_artifact_path":"/out/gate-profile-candidate.json",
						"candidate_validation_status":"valid",
						"candidate_validation_error":"",
						"candidate_promoted":false
					}
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
	if result.Jobs[0].Recovery == nil {
		t.Fatal("expected recovery payload")
	}
	if got, want := result.Jobs[0].Recovery.LoopKind, "healing"; got != want {
		t.Fatalf("loop_kind=%q, want %q", got, want)
	}
	if got, want := result.Jobs[0].Recovery.ErrorKind, "infra"; got != want {
		t.Fatalf("error_kind=%q, want %q", got, want)
	}
	if got, want := result.Jobs[0].Recovery.StrategyID, "infra-default"; got != want {
		t.Fatalf("strategy_id=%q, want %q", got, want)
	}
	if got := result.Jobs[0].Recovery.Confidence; got == nil || *got != 0.8 {
		t.Fatalf("confidence=%v, want 0.8", got)
	}
	if got, want := strings.TrimSpace(string(result.Jobs[0].Recovery.Expectations)), `{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}`; got != want {
		t.Fatalf("expectations=%q, want %q", got, want)
	}
	if got := result.Jobs[0].Recovery.CandidatePromoted; got == nil || *got {
		t.Fatalf("candidate_promoted=%v, want false", got)
	}
}
