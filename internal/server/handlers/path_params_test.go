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

	t.Run("GET /v1/runs/{id} rejects empty id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		h := getRunHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/runs/", nil)
		req.SetPathValue("id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRunCalled {
			t.Fatalf("expected no store calls, but GetRun was called")
		}
	})

	t.Run("GET /v1/runs/{id} rejects whitespace id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		h := getRunHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/runs/", nil)
		req.SetPathValue("id", "   ")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRunCalled {
			t.Fatalf("expected no store calls, but GetRun was called")
		}
	})

	t.Run("POST /v1/runs/{run_id}/jobs/{job_id}/diff rejects empty ids before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		bp := blobpersist.New(st, bsmock.New())
		h := createJobDiffHandler(st, bp)

		req := httptest.NewRequest(http.MethodPost, "/v1/runs//jobs//diff", nil)
		req.SetPathValue("run_id", "")
		req.SetPathValue("job_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getRunCalled || st.getJobCalled || st.createDiffCalled {
			t.Fatalf("expected no store calls, but got GetRun=%v GetJob=%v CreateDiff=%v", st.getRunCalled, st.getJobCalled, st.createDiffCalled)
		}
	})

	t.Run("POST /v1/migs/{mig_id}/runs rejects empty mig_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		h := createMigRunHandler(st)

		req := httptest.NewRequest(http.MethodPost, "/v1/migs//runs", nil)
		req.SetPathValue("mig_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getModCalled || st.createRunCalled {
			t.Fatalf("expected no store calls, but got GetMig=%v CreateRun=%v", st.getModCalled, st.createRunCalled)
		}
	})

	t.Run("GET /v1/repos/{repo_id}/runs rejects empty repo_id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		h := listRunsForRepoHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/repos//runs", nil)
		req.SetPathValue("repo_id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.listRunsForRepoCalled {
			t.Fatalf("expected no store calls, but ListRunsForRepo was called")
		}
	})

	t.Run("POST /v1/nodes/{id}/heartbeat rejects empty node id before store calls", func(t *testing.T) {
		t.Parallel()

		st := &mockStore{}
		h := heartbeatHandler(st)

		req := httptest.NewRequest(http.MethodPost, "/v1/nodes//heartbeat", nil)
		req.SetPathValue("id", "")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusBadRequest)
		if st.getNodeCalled || st.updateNodeHeartbeatCalled {
			t.Fatalf("expected no store calls, but got GetNode=%v UpdateNodeHeartbeat=%v", st.getNodeCalled, st.updateNodeHeartbeatCalled)
		}
	})
}
