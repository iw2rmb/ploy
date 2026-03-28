package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListJobsHandler_Success(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	nodeID := domaintypes.NodeID("abc123")

	st := &mockStore{}
	st.listJobsForTUI.val = []store.ListJobsForTUIRow{
		{
			JobID:      jobID,
			Name:       "mig-step",
			Status:     domaintypes.JobStatusRunning,
			DurationMs: 1234,
			JobImage:   "ghcr.io/iw2rmb/migs-java17:latest",
			NodeID:     &nodeID,
			MigName:    "java17-upgrade",
			RunID:      runID,
			RepoID:     repoID,
		},
		}
	st.countJobsForTUI.val = 1

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.listJobsForTUI.called {
		t.Fatal("expected ListJobsForTUI to be called")
	}
	if !st.countJobsForTUI.called {
		t.Fatal("expected CountJobsForTUI to be called")
	}

	resp := decodeBody[map[string]any](t, rr)

	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected 1 job entry, got %T len=%d", resp["jobs"], len(jobs))
	}
	job := jobs[0].(map[string]any)
	if got := job["name"]; got != "mig-step" {
		t.Fatalf("name = %v, want %q", got, "mig-step")
	}
	if got := job["mig_name"]; got != "java17-upgrade" {
		t.Fatalf("mig_name = %v, want %q", got, "java17-upgrade")
	}
	if got := job["job_id"]; got != jobID.String() {
		t.Fatalf("job_id = %v, want %q", got, jobID.String())
	}
	if got := job["run_id"]; got != runID.String() {
		t.Fatalf("run_id = %v, want %q", got, runID.String())
	}
	if got := job["repo_id"]; got != repoID.String() {
		t.Fatalf("repo_id = %v, want %q", got, repoID.String())
	}
	if got := job["status"]; got != "Running" {
		t.Fatalf("status = %v, want %q", got, "Running")
	}
	if got := job["duration_ms"]; got != float64(1234) {
		t.Fatalf("duration_ms = %v, want %d", got, 1234)
	}
	if got := job["job_image"]; got != "ghcr.io/iw2rmb/migs-java17:latest" {
		t.Fatalf("job_image = %v, want %q", got, "ghcr.io/iw2rmb/migs-java17:latest")
	}
	if got := job["node_id"]; got != "abc123" {
		t.Fatalf("node_id = %v, want %q", got, "abc123")
	}

	if got, ok := resp["total"].(float64); !ok || got != 1 {
		t.Fatalf("total = %v, want 1", resp["total"])
	}
}

func TestListJobsHandler_EmptyResult(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	st.listJobsForTUI.val = []store.ListJobsForTUIRow{}
	st.countJobsForTUI.val = 0

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)
	jobs := resp["jobs"].([]any)
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
	if got := resp["total"].(float64); got != 0 {
		t.Fatalf("total = %v, want 0", got)
	}
}

func TestListJobsHandler_RunIDFilter(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &mockStore{}
	st.listJobsForTUI.val = []store.ListJobsForTUIRow{}
	st.countJobsForTUI.val = 0

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?run_id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.listJobsForTUI.called {
		t.Fatal("expected ListJobsForTUI to be called")
	}
	if st.listJobsForTUI.params.RunID == nil || *st.listJobsForTUI.params.RunID != runID {
		t.Fatalf("expected run_id filter %q, got %v", runID, st.listJobsForTUI.params.RunID)
	}
	if st.countJobsForTUI.params == nil || *st.countJobsForTUI.params != runID {
		t.Fatalf("expected count run_id filter %q, got %v", runID, st.countJobsForTUI.params)
	}
}

func TestListJobsHandler_DefaultPagination(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	st.listJobsForTUI.val = []store.ListJobsForTUIRow{}
	st.countJobsForTUI.val = 0

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if st.listJobsForTUI.params.Limit != 50 {
		t.Fatalf("default limit = %d, want 50", st.listJobsForTUI.params.Limit)
	}
	if st.listJobsForTUI.params.Offset != 0 {
		t.Fatalf("default offset = %d, want 0", st.listJobsForTUI.params.Offset)
	}
}

func TestListJobsHandler_InvalidPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
	}{
		{"invalid limit", "/v1/jobs?limit=abc"},
		{"zero limit", "/v1/jobs?limit=0"},
		{"negative offset", "/v1/jobs?offset=-1"},
	}

	st := &mockStore{}
	handler := listJobsHandler(st)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestListJobsHandler_ListError(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	st.listJobsForTUI.err = errMockDatabase

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestListJobsHandler_CountError(t *testing.T) {
	t.Parallel()

	st := &mockStore{}
	st.listJobsForTUI.val = []store.ListJobsForTUIRow{}
	st.countJobsForTUI.err = errMockDatabase

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
}
