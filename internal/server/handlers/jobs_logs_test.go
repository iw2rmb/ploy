package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestBuildJobLogFilter_PassesRetentionEvent(t *testing.T) {
	t.Parallel()

	filter := buildJobLogFilter(map[domaintypes.JobID]struct{}{})

	data, _ := json.Marshal(map[string]any{"retained": true})
	evt := logstream.Event{Type: domaintypes.SSEEventRetention, Data: data}

	_, keep := filter(evt)
	if !keep {
		t.Fatal("expected retention event to pass job log filter")
	}
}

// --- GET /v1/jobs/{job_id}/logs backfill path (sinceID == 0) ---

// gzipLines creates gzipped data from newline-joined lines.
func gzipLines(t *testing.T, lines ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, l := range lines {
		_, _ = zw.Write([]byte(l + "\n"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGetJobLogsHandler_BackfillExcludesNilJobIDLogs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKeyJob := "logs/job.gz"
	objKeyNil := "logs/nil.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKeyJob},
		{ID: 2, RunID: runID, JobID: nil, ObjectKey: &objKeyNil}, // no job_id — must be excluded
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKeyJob, "", gzipLines(t, "job-line"))
	_, _ = bs.Put(context.Background(), objKeyNil, "", gzipLines(t, "nil-line"))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "job-line") {
		t.Fatal("expected job-line in backfill output")
	}
	if strings.Contains(body, "nil-line") {
		t.Fatal("nil-job-id log must not appear in job-scoped backfill")
	}
}

func TestGetJobLogsHandler_BackfillExcludesOtherJobLogs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	otherID := domaintypes.NewJobID()

	objKeyJob := "logs/job.gz"
	objKeyOther := "logs/other.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKeyJob},
		{ID: 2, RunID: runID, JobID: &otherID, ObjectKey: &objKeyOther},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKeyJob, "", gzipLines(t, "my-job"))
	_, _ = bs.Put(context.Background(), objKeyOther, "", gzipLines(t, "other-job"))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "my-job") {
		t.Fatal("expected my-job in backfill output")
	}
	if strings.Contains(body, "other-job") {
		t.Fatal("other job's log must not appear in job-scoped backfill")
	}
}

// TestGetJobLogsHandler_ResumeWithLastEventID verifies that GET /v1/jobs/{job_id}/logs
// with Last-Event-ID resumes from the given cursor, skipping earlier events.
func TestGetJobLogsHandler_ResumeWithLastEventID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	// Pre-publish a log event (id=1) before subscriber joins.
	ctx := context.Background()
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2025-01-01T00:00:00Z",
		Stream:    "stdout",
		Line:      "first",
		JobID:     jobID,
	})

	h := getJobLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	req.Header.Set("Last-Event-ID", "1")
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish, then publish id=2 and done.
	time.Sleep(25 * time.Millisecond)
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2025-01-01T00:00:01Z",
		Stream:    "stdout",
		Line:      "second",
		JobID:     jobID,
	})
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for resumed job stream")
	}

	body := rr.Body.String()
	if strings.Contains(body, "id: 1\n") {
		t.Fatalf("resume should not include id 1; body: %s", body)
	}
	if !strings.Contains(body, "id: 2\n") {
		t.Fatalf("resume body missing id 2: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("resume body missing done event: %s", body)
	}
}

func TestBuildJobLogFilter_RejectsNilJobIDLog(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	filter := buildJobLogFilter(map[domaintypes.JobID]struct{}{jobID: {}})

	data, _ := json.Marshal(map[string]any{"line": "hello"}) // no job_id field
	evt := logstream.Event{Type: domaintypes.SSEEventLog, Data: data}

	_, keep := filter(evt)
	if keep {
		t.Fatal("expected log event without job_id to be rejected")
	}
}
