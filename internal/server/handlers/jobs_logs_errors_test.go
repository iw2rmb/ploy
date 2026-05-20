package handlers

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// GET /v1/jobs/{job_id}/logs error/not-found paths.

func TestGetJobLogsHandler_JobNotFound(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	st.getJob.err = pgx.ErrNoRows

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	h := getJobLogsHandler(st, nil, eventsService)

	rr := doRequest(t, h, http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil, "job_id", jobID.String())
	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetJobLogsHandler_InvalidJobID(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	h := getJobLogsHandler(st, nil, eventsService)

	rr := doRequest(t, h, http.MethodGet, "/v1/jobs/invalid/logs", nil, "job_id", "invalid")
	assertStatus(t, rr, http.StatusBadRequest)
	if st.getJob.called {
		t.Fatal("expected no store calls for invalid job_id")
	}
}

func TestGetJobLogsHandler_RunNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID}
	st.getRun.err = pgx.ErrNoRows

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	h := getJobLogsHandler(st, nil, eventsService)

	rr := doRequest(t, h, http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil, "job_id", jobID.String())
	assertStatus(t, rr, http.StatusNotFound)
}
