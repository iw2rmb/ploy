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
	"github.com/iw2rmb/ploy/internal/logchunk"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// --- POST /v1/jobs/{job_id}/logs ---

func TestCreateJobLogsHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupStore     func(jobID domaintypes.JobID, runID domaintypes.RunID) *jobStore
		payload        map[string]any
		wantStatus     int
		wantGetJobCall bool
	}{
		{
			name: "success",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *jobStore {
				objKey := "logs/job/" + jobID.String() + "/log/1.gz"
				st := &jobStore{getJobResult: store.Job{ID: jobID, RunID: runID}}
				st.createLog.val = store.Log{ID: 1, RunID: runID, JobID: &jobID, ChunkNo: 2, DataSize: 5, ObjectKey: &objKey}
				return st
			},
			payload:        map[string]any{"chunk_no": 2, "data": []byte("hello")},
			wantStatus:     http.StatusCreated,
			wantGetJobCall: true,
		},
		{
			name: "job not found",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *jobStore {
				return &jobStore{getJobErr: pgx.ErrNoRows}
			},
			payload:        map[string]any{"chunk_no": 0, "data": []byte("x")},
			wantStatus:     http.StatusNotFound,
			wantGetJobCall: true,
		},
		{
			name: "empty data",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *jobStore {
				return &jobStore{getJobResult: store.Job{ID: jobID, RunID: runID}}
			},
			payload:        map[string]any{"chunk_no": 0, "data": []byte{}},
			wantStatus:     http.StatusBadRequest,
			wantGetJobCall: true,
		},
		{
			name: "too large",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *jobStore {
				return &jobStore{getJobResult: store.Job{ID: jobID, RunID: runID}}
			},
			payload:        map[string]any{"chunk_no": 0, "data": make([]byte, 10<<20+1)},
			wantStatus:     http.StatusRequestEntityTooLarge,
			wantGetJobCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := domaintypes.NewRunID()
			jobID := domaintypes.NewJobID()
			st := tt.setupStore(jobID, runID)

			eventsService, err := createTestEventsServiceWithStore(st)
			if err != nil {
				t.Fatalf("events service: %v", err)
			}
			h := createJobLogsHandler(st, blobpersist.New(st, bsmock.New()), eventsService)

			b, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", bytes.NewReader(b))
			req.SetPathValue("job_id", jobID.String())
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if st.getJobCalled != tt.wantGetJobCall {
				t.Fatalf("getJobCalled = %v, want %v", st.getJobCalled, tt.wantGetJobCall)
			}
		})
	}
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

// --- GET /v1/jobs/{job_id}/logs backfill path (sinceID == 0) ---

// gzipLines creates gzipped data from newline-joined lines.
func gzipLines(t *testing.T, lines ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if err := logchunk.EncodeRecordLine(zw, logchunk.StreamStdout, l); err != nil {
			t.Fatalf("encode framed line: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gzipFrames(t *testing.T, records ...logchunk.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, record := range records {
		if strings.TrimSpace(record.Line) == "" {
			continue
		}
		if err := logchunk.EncodeRecordLine(zw, record.Stream, record.Line); err != nil {
			t.Fatalf("encode framed line: %v", err)
		}
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

func TestGetJobLogsHandler_BackfillPreservesStderrStream(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	objKey := "logs/job-stderr.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKey, "", gzipFrames(t,
		logchunk.Record{Stream: logchunk.StreamStdout, Line: "out-line"},
		logchunk.Record{Stream: logchunk.StreamStderr, Line: "err-line"},
	))

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
	if !strings.Contains(body, `"stream":"stdout","line":"out-line"`) {
		t.Fatalf("expected stdout frame in output; body: %s", body)
	}
	if !strings.Contains(body, `"stream":"stderr","line":"err-line"`) {
		t.Fatalf("expected stderr frame in output; body: %s", body)
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

// TestGetJobLogsHandler_RetentionFrame verifies that GET /v1/jobs/{job_id}/logs
// emits retention events published on the job stream.
func TestGetJobLogsHandler_RetentionFrame(t *testing.T) {
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

	h := getJobLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish, then publish retention + done.
	time.Sleep(25 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishJobRetention(ctx, jobID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2026-04-08T00:00:00Z",
	})
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for job stream with retention")
	}

	body := rr.Body.String()
	if !strings.Contains(body, "event: retention") {
		t.Fatalf("expected retention event on job stream; body: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event on job stream; body: %s", body)
	}
}

// TestGetJobLogsHandler_BackfillLiveNoDuplicates verifies that events published
// to the hub during backfill are not delivered twice (once via backfill, once via
// live subscription) and that ordering is backfill-first then live.
func TestGetJobLogsHandler_BackfillLiveNoDuplicates(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/job.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKey, "", gzipLines(t, "backfill-line"))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	// Pre-publish a log event to the hub BEFORE the handler runs.
	// This simulates an event published during backfill that overlaps with DB content.
	ctx := context.Background()
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:00Z",
		Stream:    "stdout",
		Line:      "hub-overlap-line",
		JobID:     jobID,
	})

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait for handler to establish, then publish live events.
	time.Sleep(50 * time.Millisecond)
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:01Z",
		Stream:    "stdout",
		Line:      "live-line",
		JobID:     jobID,
	})
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for backfill+live stream")
	}

	body := rr.Body.String()

	// Backfill content must appear.
	if !strings.Contains(body, "backfill-line") {
		t.Fatalf("expected backfill-line in output; body: %s", body)
	}
	// Live content must appear.
	if !strings.Contains(body, "live-line") {
		t.Fatalf("expected live-line in output; body: %s", body)
	}
	// Hub overlap line (published before backfill, a log event) must NOT be
	// replayed as a live event — it's covered by backfill from DB.
	// Count occurrences of "hub-overlap-line" — should be zero (it's in hub, not DB backfill).
	if strings.Count(body, "hub-overlap-line") > 0 {
		t.Fatalf("hub log event published during backfill should be deduped; body: %s", body)
	}
	// Ordering: backfill content must appear before live content.
	backfillIdx := strings.Index(body, "backfill-line")
	liveIdx := strings.Index(body, "live-line")
	if backfillIdx > liveIdx {
		t.Fatalf("backfill must precede live; backfill@%d live@%d; body: %s", backfillIdx, liveIdx, body)
	}
	// Done sentinel must be present.
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}

// TestGetJobLogsHandler_GapLogEventsDelivered verifies that log events published
// to the hub DURING backfill (after preCursor, before subscription) are delivered
// to the client rather than being silently dropped.
func TestGetJobLogsHandler_GapLogEventsDelivered(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/gap-job.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKey, "", gzipLines(t, "backfill-line"))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait for handler to start backfill, then publish a log event that lands
	// in the gap (after preCursor snapshot but before live subscription).
	time.Sleep(50 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:00Z",
		Stream:    "stdout",
		Line:      "gap-log-line",
		JobID:     jobID,
	})

	// Small delay, then publish another live event and done to terminate stream.
	time.Sleep(20 * time.Millisecond)
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:01Z",
		Stream:    "stdout",
		Line:      "live-line",
		JobID:     jobID,
	})
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for gap log delivery stream")
	}

	body := rr.Body.String()

	// Backfill content from DB must appear.
	if !strings.Contains(body, "backfill-line") {
		t.Fatalf("expected backfill-line in output; body: %s", body)
	}
	// Gap log event must NOT be dropped — it was published during backfill
	// and is not in the DB, so it must be replayed from the hub gap.
	if !strings.Contains(body, "gap-log-line") {
		t.Fatalf("gap log event published during backfill must be delivered; body: %s", body)
	}
	// Live content must appear.
	if !strings.Contains(body, "live-line") {
		t.Fatalf("expected live-line in output; body: %s", body)
	}
	// Ordering: backfill < gap < live.
	backfillIdx := strings.Index(body, "backfill-line")
	gapIdx := strings.Index(body, "gap-log-line")
	liveIdx := strings.Index(body, "live-line")
	if backfillIdx > gapIdx || gapIdx > liveIdx {
		t.Fatalf("expected backfill < gap < live ordering; backfill@%d gap@%d live@%d; body: %s",
			backfillIdx, gapIdx, liveIdx, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}

// TestGetJobLogsHandler_OverlapDedupDuringBackfill verifies that a log line
// persisted to the DB AND published to the hub during the backfill window is
// emitted only once. Without dedup, the client would see the line from
// backfill (via ListLogsByRun) and again from gap replay (hub ID > preCursor).
func TestGetJobLogsHandler_OverlapDedupDuringBackfill(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	// The overlap line will appear in both the DB backfill chunk and the hub.
	overlapLine := "overlap-persisted-and-hub"
	objKey := "logs/overlap-job.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	// Chunk contains the overlap line (simulating it was persisted to DB
	// during the backfill window).
	_, _ = bs.Put(context.Background(), objKey, "", gzipLines(t, overlapLine))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Wait for handler to start backfill, then publish the SAME line to the
	// hub so it lands in the gap window (hub ID > preCursor).
	time.Sleep(50 * time.Millisecond)
	ctx := context.Background()
	// Timestamp must be empty to match the backfill timestamp (CreatedAt is
	// zero-valued, so timestampToString returns ""). The composite dedup key
	// includes timestamp+stream+line, so all three must match for dedup.
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "",
		Stream:    "stdout",
		Line:      overlapLine,
		JobID:     jobID,
	})

	// Publish a distinct live line and done to terminate the stream.
	time.Sleep(20 * time.Millisecond)
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:01Z",
		Stream:    "stdout",
		Line:      "unique-live-line",
		JobID:     jobID,
	})
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for overlap dedup stream")
	}

	body := rr.Body.String()

	// The overlap line MUST appear exactly once — from backfill, not duplicated
	// by gap replay.
	if count := strings.Count(body, overlapLine); count != 1 {
		t.Fatalf("overlap line should appear exactly once, got %d; body: %s", count, body)
	}
	// Unique live line must still be delivered.
	if !strings.Contains(body, "unique-live-line") {
		t.Fatalf("expected unique-live-line in output; body: %s", body)
	}
	// Ordering: overlap (from backfill) before unique live.
	overlapIdx := strings.Index(body, overlapLine)
	liveIdx := strings.Index(body, "unique-live-line")
	if overlapIdx > liveIdx {
		t.Fatalf("backfill overlap must precede live; overlap@%d live@%d; body: %s",
			overlapIdx, liveIdx, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}

// TestGetJobLogsHandler_RepeatedLiveLineNotDropped verifies that a live log line
// whose content matches a previously backfilled line but has a different timestamp
// is NOT incorrectly deduplicated. Only true overlaps (same timestamp+stream+line)
// should be suppressed.
func TestGetJobLogsHandler_RepeatedLiveLineNotDropped(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	repeatedLine := "repeated-content"
	objKey := "logs/repeated-job.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusStarted}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKey, "", gzipLines(t, repeatedLine))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	ctx := context.Background()

	// Publish a live line with the SAME content but a DIFFERENT timestamp.
	// This is a legitimate new log emission, not a true overlap, so it must
	// NOT be deduplicated.
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-06-01T12:00:00Z",
		Stream:    "stdout",
		Line:      repeatedLine,
		JobID:     jobID,
	})

	time.Sleep(20 * time.Millisecond)
	_ = hub.PublishJobStatus(ctx, jobID, logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for repeated-line stream")
	}

	body := rr.Body.String()

	// The repeated line must appear TWICE: once from backfill, once from live.
	if count := strings.Count(body, repeatedLine); count != 2 {
		t.Fatalf("repeated line should appear exactly twice (backfill + live), got %d; body: %s", count, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}

// TestGetJobLogsHandler_TerminalBackfillIncludesRetention verifies that a fresh
// connection (sinceID == 0) to a terminal job with blobstore configured emits
// retention frames that were already published to the hub before the handler ran.
func TestGetJobLogsHandler_TerminalBackfillIncludesRetention(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/terminal-job.gz"

	st := &jobStore{
		getJobResult: store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.Background(), objKey, "", gzipLines(t, "terminal-line"))

	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	hub := eventsService.Hub()

	// Publish a retention event to the hub BEFORE the handler runs.
	ctx := context.Background()
	_ = hub.PublishJobRetention(ctx, jobID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2026-04-08T00:00:00Z",
	})

	h := getJobLogsHandler(st, bs, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID.String()+"/logs", nil)
	req.SetPathValue("job_id", jobID.String())
	rr := &flushRecorder{httptest.NewRecorder()}

	h.ServeHTTP(rr, req)

	body := rr.Body.String()

	if !strings.Contains(body, "terminal-line") {
		t.Fatalf("expected terminal-line in backfill output; body: %s", body)
	}
	if !strings.Contains(body, "event: retention") {
		t.Fatalf("terminal backfill must include retention event; body: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
	retIdx := strings.Index(body, "event: retention")
	doneIdx := strings.Index(body, "event: done")
	if retIdx > doneIdx {
		t.Fatalf("retention must precede done; retention@%d done@%d; body: %s", retIdx, doneIdx, body)
	}
}
