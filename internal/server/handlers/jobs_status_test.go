package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestGetJobStatusHandler_Success(t *testing.T) {
	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getJobResult: f.Job,
	}

	handler := getJobStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+f.JobID.String()+"/status", nil)
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	assertJSONValue(t, rr.Body.String(), "job_id", f.JobID.String())
	assertJSONValue(t, rr.Body.String(), "status", string(store.JobStatusRunning))
}

func TestGetJobStatusHandler_ForbiddenWhenNodeMismatched(t *testing.T) {
	f := newJobFixture("mig", 1000)
	st := &mockStore{
		getJobResult: f.Job,
	}

	handler := getJobStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+f.JobID.String()+"/status", nil)
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, domaintypes.NewNodeKey())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestGetJobStatusHandler_NotFound(t *testing.T) {
	jobID := domaintypes.NewJobID()
	st := &mockStore{
		getJobErr: pgx.ErrNoRows,
	}

	handler := getJobStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/status", nil)
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, domaintypes.NewNodeKey())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestGetJobStatusHandler_StoreError(t *testing.T) {
	jobID := domaintypes.NewJobID()
	st := &mockStore{
		getJobErr: errors.New("db down"),
	}

	handler := getJobStatusHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/status", nil)
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set(nodeUUIDHeader, domaintypes.NewNodeKey())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
}

func assertJSONValue(t *testing.T, body, key, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got, _ := payload[key].(string)
	if got != want {
		t.Fatalf("response[%q] = %q, want %q", key, got, want)
	}
}
