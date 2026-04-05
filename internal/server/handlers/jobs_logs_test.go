package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// --- POST /v1/jobs/{job_id}/logs ---

func TestCreateJobLogsHandler_Success(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "logs/job/" + jobID.String() + "/log/1.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID},
	}
	st.createLog.val = store.Log{ID: 1, RunID: runID, JobID: &jobID, ChunkNo: 2, DataSize: 5, ObjectKey: &objKey}

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createJobLogsHandler(st, bp, eventsService)

	payload := map[string]any{"chunk_no": 2, "data": []byte("hello")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusCreated)
	assertCalled(t, "GetJob", st.getJobCalled)
}

func TestCreateJobLogsHandler_JobNotFound(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	st := &jobStore{getJobErr: pgx.ErrNoRows}

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createJobLogsHandler(st, bp, eventsService)

	payload := map[string]any{"chunk_no": 0, "data": []byte("x")}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestCreateJobLogsHandler_EmptyData(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID},
	}

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createJobLogsHandler(st, bp, eventsService)

	payload := map[string]any{"chunk_no": 0, "data": []byte{}}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateJobLogsHandler_TooLarge(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID},
	}

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	bp := blobpersist.New(st, bsmock.New())
	h := createJobLogsHandler(st, bp, eventsService)

	big := make([]byte, 10<<20+1)
	payload := map[string]any{"chunk_no": 0, "data": big}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", bytes.NewReader(b))
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusRequestEntityTooLarge)
}

// --- GET /v1/jobs/{job_id}/logs ---

func TestGetJobLogsHandler_JobNotFound(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	st := &jobStore{getJobErr: pgx.ErrNoRows}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	h := getJobLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

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

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/invalid/logs", nil)
	req.SetPathValue("job_id", "invalid")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	if st.getJobCalled {
		t.Fatal("expected no store calls for invalid job_id")
	}
}

func TestGetJobLogsHandler_RunNotFound(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID},
	}
	st.getRun.err = pgx.ErrNoRows

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	h := getJobLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNotFound)
}

// --- buildJobLogFilter ---

func TestBuildJobLogFilter_PassesMatchingLog(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	allowed := map[domaintypes.JobID]struct{}{jobID: {}}
	filter := buildJobLogFilter(allowed)

	jobIDStr := jobID.String()
	data, _ := json.Marshal(map[string]any{"job_id": jobIDStr, "line": "hello"})
	evt := logstream.Event{Type: domaintypes.SSEEventLog, Data: data}

	out, keep := filter(evt)
	if !keep {
		t.Fatal("expected log event to pass filter")
	}
	if out.Type != domaintypes.SSEEventLog {
		t.Fatalf("expected event type %q, got %q", domaintypes.SSEEventLog, out.Type)
	}
}

func TestBuildJobLogFilter_RejectsNonMatchingLog(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	otherID := domaintypes.NewJobID()
	allowed := map[domaintypes.JobID]struct{}{jobID: {}}
	filter := buildJobLogFilter(allowed)

	otherStr := otherID.String()
	data, _ := json.Marshal(map[string]any{"job_id": otherStr, "line": "hello"})
	evt := logstream.Event{Type: domaintypes.SSEEventLog, Data: data}

	_, keep := filter(evt)
	if keep {
		t.Fatal("expected log event for other job to be rejected")
	}
}

func TestBuildJobLogFilter_PassesDoneEvent(t *testing.T) {
	t.Parallel()

	filter := buildJobLogFilter(map[domaintypes.JobID]struct{}{})

	data, _ := json.Marshal(map[string]string{"status": "done"})
	evt := logstream.Event{Type: domaintypes.SSEEventDone, Data: data}

	_, keep := filter(evt)
	if !keep {
		t.Fatal("expected done event to pass filter")
	}
}

func TestBuildJobLogFilter_RejectsRunEvent(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	filter := buildJobLogFilter(map[domaintypes.JobID]struct{}{jobID: {}})

	data, _ := json.Marshal(map[string]string{"state": "running"})
	evt := logstream.Event{Type: domaintypes.SSEEventRun, Data: data}

	_, keep := filter(evt)
	if keep {
		t.Fatal("expected run event to be rejected from job log stream")
	}
}

func TestBuildJobLogFilter_RejectsRetentionEvent(t *testing.T) {
	t.Parallel()

	filter := buildJobLogFilter(map[domaintypes.JobID]struct{}{})

	data, _ := json.Marshal(map[string]any{"retained": true})
	evt := logstream.Event{Type: domaintypes.SSEEventRetention, Data: data}

	_, keep := filter(evt)
	if keep {
		t.Fatal("expected retention event to be rejected from job log stream")
	}
}
