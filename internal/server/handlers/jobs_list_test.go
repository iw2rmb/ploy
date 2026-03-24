package handlers

import (
	"encoding/json"
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

	st := &mockStore{
		listJobsForTUIResult: []store.ListJobsForTUIRow{
			{
				JobID:   jobID,
				Name:    "mig-step",
				MigName: "java17-upgrade",
				RunID:   runID,
				RepoID:  repoID,
			},
		},
		countJobsForTUIResult: 1,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.listJobsForTUICalled {
		t.Fatal("expected ListJobsForTUI to be called")
	}
	if !st.countJobsForTUICalled {
		t.Fatal("expected CountJobsForTUI to be called")
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

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

	if got, ok := resp["total"].(float64); !ok || got != 1 {
		t.Fatalf("total = %v, want 1", resp["total"])
	}
}

func TestListJobsHandler_EmptyResult(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listJobsForTUIResult:  []store.ListJobsForTUIRow{},
		countJobsForTUIResult: 0,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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
	st := &mockStore{
		listJobsForTUIResult:  []store.ListJobsForTUIRow{},
		countJobsForTUIResult: 0,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?run_id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.listJobsForTUICalled {
		t.Fatal("expected ListJobsForTUI to be called")
	}
	if st.listJobsForTUIParams.RunID == nil || *st.listJobsForTUIParams.RunID != runID {
		t.Fatalf("expected run_id filter %q, got %v", runID, st.listJobsForTUIParams.RunID)
	}
	if st.countJobsForTUIParam == nil || *st.countJobsForTUIParam != runID {
		t.Fatalf("expected count run_id filter %q, got %v", runID, st.countJobsForTUIParam)
	}
}

func TestListJobsHandler_DefaultPagination(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listJobsForTUIResult:  []store.ListJobsForTUIRow{},
		countJobsForTUIResult: 0,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if st.listJobsForTUIParams.Limit != 50 {
		t.Fatalf("default limit = %d, want 50", st.listJobsForTUIParams.Limit)
	}
	if st.listJobsForTUIParams.Offset != 0 {
		t.Fatalf("default offset = %d, want 0", st.listJobsForTUIParams.Offset)
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
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("%s: expected status 400, got %d", tc.name, rr.Code)
			}
		})
	}
}

func TestListJobsHandler_ListError(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listJobsForTUIErr: errMockDatabase,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

func TestListJobsHandler_CountError(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		listJobsForTUIResult: []store.ListJobsForTUIRow{},
		countJobsForTUIErr:   errMockDatabase,
	}

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
