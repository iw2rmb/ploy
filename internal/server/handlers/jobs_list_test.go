package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListJobsHandler_Success(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	nodeID := domaintypes.NodeID("abc123")
	startedAt := time.Date(2026, 11, 20, 15, 32, 21, 0, time.FixedZone("MSK", 3*60*60))
	finishedAt := startedAt.Add(1234 * time.Millisecond)

	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{
		{
			JobID:      jobID,
			Name:       "mig-step",
			JobType:    domaintypes.JobTypeMig,
			Status:     domaintypes.JobStatusSuccess,
			Meta:       []byte(`{"kind":"mig","mig_step_name":"deprecations"}`),
			StartedAt:  pgtype.Timestamptz{Time: startedAt, Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
			DurationMs: 1234,
			JobImage:   "ghcr.io/iw2rmb/ploy/migs-java17:latest",
			NodeID:     &nodeID,
			MigName:    "java17-upgrade",
			RunID:      runID,
			RepoID:     repoID,
		},
	}
	st.countJobsPage.val = 1

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.listJobsPage.called {
		t.Fatal("expected ListJobsPage to be called")
	}
	if !st.countJobsPage.called {
		t.Fatal("expected CountJobsPage to be called")
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
	if got := job["display_name"]; got != "deprecations" {
		t.Fatalf("display_name = %v, want deprecations", got)
	}
	if got := job["started_at"]; got != startedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("started_at = %v, want %s", got, startedAt.UTC().Format(time.RFC3339Nano))
	}
	if got := job["finished_at"]; got != finishedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("finished_at = %v, want %s", got, finishedAt.UTC().Format(time.RFC3339Nano))
	}
	if got := job["job_type"]; got != "mig" {
		t.Fatalf("job_type = %v, want %q", got, "mig")
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
	if got := job["status"]; got != "Success" {
		t.Fatalf("status = %v, want %q", got, "Success")
	}
	if got := job["duration_ms"]; got != float64(1234) {
		t.Fatalf("duration_ms = %v, want %d", got, 1234)
	}
	if got := job["job_image"]; got != "ghcr.io/iw2rmb/ploy/migs-java17:latest" {
		t.Fatalf("job_image = %v, want %q", got, "ghcr.io/iw2rmb/ploy/migs-java17:latest")
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

	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{}
	st.countJobsPage.val = 0

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
	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{}
	st.countJobsPage.val = 0

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?run_id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.listJobsPage.called {
		t.Fatal("expected ListJobsPage to be called")
	}
	if st.listJobsPage.params.RunID == nil || *st.listJobsPage.params.RunID != runID.String() {
		t.Fatalf("expected run_id filter %q, got %v", runID, st.listJobsPage.params.RunID)
	}
	if st.countJobsPage.params.RunID == nil || *st.countJobsPage.params.RunID != runID.String() {
		t.Fatalf("expected count run_id filter %q, got %v", runID, st.countJobsPage.params.RunID)
	}
}

func TestListJobsHandler_NodeAndStatusFilters(t *testing.T) {
	t.Parallel()

	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{}
	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs?node_id=abc123&status=Running", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if got := st.listJobsPage.params.NodeID; got == nil || *got != "abc123" {
		t.Fatalf("list node_id = %v, want abc123", got)
	}
	if got := st.listJobsPage.params.Status; got == nil || *got != "Running" {
		t.Fatalf("list status = %v, want Running", got)
	}
	if got := st.countJobsPage.params.NodeID; got == nil || *got != "abc123" {
		t.Fatalf("count node_id = %v, want abc123", got)
	}
	if got := st.countJobsPage.params.Status; got == nil || *got != "Running" {
		t.Fatalf("count status = %v, want Running", got)
	}
}

func TestListJobsHandler_DefaultPagination(t *testing.T) {
	t.Parallel()

	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{}
	st.countJobsPage.val = 0

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if st.listJobsPage.params.Limit != 50 {
		t.Fatalf("default limit = %d, want 50", st.listJobsPage.params.Limit)
	}
	if st.listJobsPage.params.Offset != 0 {
		t.Fatalf("default offset = %d, want 0", st.listJobsPage.params.Offset)
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

	st := &handlerStore{}
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

func TestListJobsHandler_InvalidFilters(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/v1/jobs?node_id=invalid%20node",
		"/v1/jobs?status=running",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		listJobsHandler(&handlerStore{}).ServeHTTP(rr, req)
		assertStatus(t, rr, http.StatusBadRequest)
	}
}

func TestListJobsHandler_ListError(t *testing.T) {
	t.Parallel()

	st := &handlerStore{}
	st.listJobsPage.err = errMockDatabase

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestListJobsHandler_CountError(t *testing.T) {
	t.Parallel()

	st := &handlerStore{}
	st.listJobsPage.val = []store.ListJobsPageRow{}
	st.countJobsPage.err = errMockDatabase

	handler := listJobsHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestListJobsHandlerOptionalPresentationMetadata(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 11, 20, 12, 36, 33, 0, time.UTC)
	for _, tt := range []struct {
		name      string
		meta      []byte
		status    domaintypes.JobStatus
		startedAt pgtype.Timestamptz
		wantName  string
	}{
		{
			name:      "running step exposes display name and start time",
			meta:      []byte(`{"kind":"mig","mig_step_name":"healing"}`),
			status:    domaintypes.JobStatusRunning,
			startedAt: pgtype.Timestamptz{Time: startedAt, Valid: true},
			wantName:  "healing",
		},
		{
			name:   "pending unnamed step omits optional fields",
			meta:   []byte(`{"kind":"mig","mig_step_index":2}`),
			status: domaintypes.JobStatusCreated,
		},
		{
			name:   "invalid metadata preserves raw name and type",
			meta:   []byte(`{"mig_step_name":"ignored"}`),
			status: domaintypes.JobStatusCreated,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &handlerStore{}
			st.listJobsPage.val = []store.ListJobsPageRow{{
				JobID: domaintypes.NewJobID(), Name: "mig-2", JobType: domaintypes.JobTypeMig,
				RunID: domaintypes.NewRunID(), RepoID: domaintypes.NewRepoID(),
				Status: tt.status, Meta: tt.meta, StartedAt: tt.startedAt,
			}}
			st.countJobsPage.val = 1
			rr := httptest.NewRecorder()
			listJobsHandler(st).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/jobs", nil))
			assertStatus(t, rr, http.StatusOK)
			resp := decodeBody[domainapi.JobListResponse](t, rr)
			if len(resp.Jobs) != 1 {
				t.Fatalf("jobs count = %d, want 1", len(resp.Jobs))
			}
			job := resp.Jobs[0]
			if job.Name != "mig-2" || job.JobType != domaintypes.JobTypeMig || job.DisplayName != tt.wantName {
				t.Fatalf("job names = %q/%q/%q, want mig-2/mig/%q", job.Name, job.JobType, job.DisplayName, tt.wantName)
			}
			if tt.startedAt.Valid {
				if job.StartedAt == nil || !job.StartedAt.Equal(startedAt) {
					t.Fatalf("started_at = %v, want %v", job.StartedAt, startedAt)
				}
			} else if job.StartedAt != nil {
				t.Fatalf("started_at = %v, want nil", job.StartedAt)
			}
			if job.FinishedAt != nil {
				t.Fatalf("finished_at = %v, want nil", job.FinishedAt)
			}
		})
	}
}

// Creator filtering uses the same selection for the page and total, and requires cli-admin.
func TestListJobsHandlerCreatorFilter(t *testing.T) {
	for _, tc := range []struct {
		name        string
		role        auth.Role
		query       string
		wantStatus  int
		wantCreator string
	}{
		{name: "admin creator", role: auth.RoleCLIAdmin, query: "&creator=%20alice%20", wantStatus: http.StatusOK, wantCreator: "alice"},
		{name: "admin all", role: auth.RoleCLIAdmin, wantStatus: http.StatusOK},
		{name: "control-plane all", role: auth.RoleControlPlane, wantStatus: http.StatusOK},
		{name: "control-plane creator", role: auth.RoleControlPlane, query: "&creator=alice", wantStatus: http.StatusForbidden},
		{name: "worker creator", role: auth.RoleWorker, query: "&creator=alice", wantStatus: http.StatusForbidden},
		{name: "missing identity creator", query: "&creator=alice", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &handlerStore{}
			req := httptest.NewRequest(http.MethodGet, "/v1/jobs?limit=5&offset=10"+tc.query, nil)
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: tc.role}))
			rr := httptest.NewRecorder()
			listJobsHandler(st).ServeHTTP(rr, req)
			assertStatus(t, rr, tc.wantStatus)
			if tc.wantStatus == http.StatusForbidden {
				if st.listJobsPage.called || st.countJobsPage.called {
					t.Fatal("unauthorized creator filter reached the store")
				}
				return
			}
			if !st.listJobsPage.called || !st.countJobsPage.called {
				t.Fatal("page and count were not both queried")
			}
			for name, got := range map[string]*string{"page": st.listJobsPage.params.CreatedBy, "count": st.countJobsPage.params.CreatedBy} {
				if tc.wantCreator == "" {
					if got != nil {
						t.Fatalf("%s creator = %q, want no filter", name, *got)
					}
				} else if got == nil || *got != tc.wantCreator {
					t.Fatalf("%s creator = %v, want %s", name, got, tc.wantCreator)
				}
			}
			if st.listJobsPage.params.Limit != 5 || st.listJobsPage.params.Offset != 10 {
				t.Fatalf("creator filtering lost pagination: %+v", st.listJobsPage.params)
			}
		})
	}
}
