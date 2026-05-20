package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
	"github.com/iw2rmb/ploy/internal/store"
)

// GET /v1/jobs/{job_id}/logs backfill + live SSE tests.
// POST handler tests live in jobs_logs_create_test.go.
// GET error/not-found tests live in jobs_logs_errors_test.go.

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

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKeyJob},
		{ID: 2, RunID: runID, JobID: nil, ObjectKey: &objKeyNil}, // no job_id; must be excluded
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

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess}
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

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess}
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
