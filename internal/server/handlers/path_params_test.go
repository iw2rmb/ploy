package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
)

// TestPathParamsUseDomainTypes validates that handlers reject invalid/blank
// path IDs with a 400 before making any store calls.
func TestPathParamsUseDomainTypes(t *testing.T) {
	t.Parallel()

	t.Run("GET /v1/runs/{run_id} rejects empty id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &runStore{}
		h := getRunHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/runs/", nil)
		req.SetPathValue("run_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRun.called {
			t.Fatalf("expected no store calls, but GetRun was called")
		}
	})

	t.Run("GET /v1/runs/{run_id} rejects whitespace id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &runStore{}
		h := getRunHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/runs/", nil)
		req.SetPathValue("run_id", "   ")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRun.called {
			t.Fatalf("expected no store calls, but GetRun was called")
		}
	})

	t.Run("POST /v1/runs/{run_id}/jobs/{job_id}/diff rejects empty ids before store calls", func(t *testing.T) {
		t.Parallel()

		st := &runStore{}
		bp := blobpersist.New(st, bsmock.New())
		h := createJobDiffHandler(st, bp)

		req := httptest.NewRequest(http.MethodPost, "/v1/runs//jobs//diff", nil)
		req.SetPathValue("run_id", "")
		req.SetPathValue("job_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRun.called || st.getJobCalled || st.createDiff.called {
			t.Fatalf("expected no store calls, but got GetRun=%v GetJob=%v CreateDiff=%v", st.getRun.called, st.getJobCalled, st.createDiff.called)
		}
	})

	t.Run("POST /v1/migs/{mig_id}/runs rejects empty mig_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &migStore{}
		h := createMigRunHandler(st)

		req := httptest.NewRequest(http.MethodPost, "/v1/migs//runs", nil)
		req.SetPathValue("mig_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getMigCalled || st.createRunCalled {
			t.Fatalf("expected no store calls, but got GetMig=%v CreateRun=%v", st.getMigCalled, st.createRunCalled)
		}
	})

	t.Run("GET /v1/repos/{repo_id}/runs rejects empty repo_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &repoListStore{}
		h := listRunsForRepoHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/repos//runs", nil)
		req.SetPathValue("repo_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.listRunsForRepo.called {
			t.Fatalf("expected no store calls, but ListRunsForRepo was called")
		}
	})

	t.Run("GET /v1/jobs/{job_id}/logs rejects empty job_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &jobStore{}
		eventsService, err := createTestEventsService()
		if err != nil {
			t.Fatalf("events service: %v", err)
		}
		h := getJobLogsHandler(st, nil, eventsService)

		req := httptest.NewRequest(http.MethodGet, "/v1/jobs//logs", nil)
		req.SetPathValue("job_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getJobCalled {
			t.Fatalf("expected no store calls, but GetJob was called")
		}
	})

	t.Run("POST /v1/jobs/{job_id}/logs rejects empty job_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &jobStore{}
		eventsService, err := createTestEventsServiceWithStore(st)
		if err != nil {
			t.Fatalf("events service: %v", err)
		}
		bp := blobpersist.New(st, bsmock.New())
		h := createJobLogsHandler(st, bp, eventsService)

		req := httptest.NewRequest(http.MethodPost, "/v1/jobs//logs", nil)
		req.SetPathValue("job_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getJobCalled {
			t.Fatalf("expected no store calls, but GetJob was called")
		}
	})

	t.Run("POST /v1/nodes/{id}/heartbeat rejects empty node id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &nodeStore{}
		h := heartbeatHandler(st)

		req := httptest.NewRequest(http.MethodPost, "/v1/nodes//heartbeat", nil)
		req.SetPathValue("id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getNode.called || st.updateNodeHeartbeat.called {
			t.Fatalf("expected no store calls, but got GetNode=%v UpdateNodeHeartbeat=%v", st.getNode.called, st.updateNodeHeartbeat.called)
		}
	})
}
