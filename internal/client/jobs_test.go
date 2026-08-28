package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestListJobsCommandSendsFiltersAndDecodesResponse(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	nodeID := domaintypes.NodeID("abc123")
	status := domaintypes.JobStatusRunning
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs" {
			t.Fatalf("path = %q, want /api/v1/jobs", r.URL.Path)
		}
		for key, want := range map[string]string{
			"limit": "5", "offset": "10", "run_id": runID.String(),
			"node_id": nodeID.String(), "status": status.String(),
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{{
				"job_id": jobID, "name": "pre-gate", "job_type": "pre_gate",
				"status": status, "duration_ms": 0, "job_image": "gate:latest",
				"node_id": nodeID, "mig_name": "upgrade", "run_id": runID, "repo_id": repoID,
			}},
			"total": 1,
		})
	}))
	t.Cleanup(server.Close)
	baseURL, _ := url.Parse(server.URL + "/api")

	result, err := (ListJobsCommand{
		Client: server.Client(), BaseURL: baseURL, Limit: 5, Offset: 10,
		RunID: &runID, NodeID: &nodeID, Status: &status,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Total != 1 || len(result.Jobs) != 1 || result.Jobs[0].JobID != jobID {
		t.Fatalf("Run() = %+v, want one job %s", result, jobID)
	}
}

func TestListJobsCommandRequiresClientAndURL(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("http://localhost")
	if _, err := (ListJobsCommand{BaseURL: baseURL}).Run(context.Background()); err == nil {
		t.Fatal("expected missing-client error")
	}
	if _, err := (ListJobsCommand{Client: http.DefaultClient}).Run(context.Background()); err == nil {
		t.Fatal("expected missing-base-url error")
	}
}
