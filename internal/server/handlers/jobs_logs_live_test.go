package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// TestGetJobLogsHandler_ResumeWithLastEventID verifies that GET /v1/jobs/{job_id}/logs
// with Last-Event-ID resumes from the given cursor, skipping earlier events.
func TestGetJobLogsHandler_ResumeWithLastEventID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}

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

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}

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
// to the hub during backfill are not delivered twice and that ordering is
// backfill-first then live.
func TestGetJobLogsHandler_BackfillLiveNoDuplicates(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/job.gz"

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}
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
	if !strings.Contains(body, "backfill-line") {
		t.Fatalf("expected backfill-line in output; body: %s", body)
	}
	if !strings.Contains(body, "live-line") {
		t.Fatalf("expected live-line in output; body: %s", body)
	}
	if strings.Count(body, "hub-overlap-line") > 0 {
		t.Fatalf("hub log event published during backfill should be deduped; body: %s", body)
	}
	backfillIdx := strings.Index(body, "backfill-line")
	liveIdx := strings.Index(body, "live-line")
	if backfillIdx > liveIdx {
		t.Fatalf("backfill must precede live; backfill@%d live@%d; body: %s", backfillIdx, liveIdx, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}

// TestGetJobLogsHandler_GapLogEventsDelivered verifies that log events published
// to the hub during backfill are delivered to the client.
func TestGetJobLogsHandler_GapLogEventsDelivered(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/gap-job.gz"

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}
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

	time.Sleep(50 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "2026-01-01T00:00:00Z",
		Stream:    "stdout",
		Line:      "gap-log-line",
		JobID:     jobID,
	})

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
	if !strings.Contains(body, "backfill-line") {
		t.Fatalf("expected backfill-line in output; body: %s", body)
	}
	if !strings.Contains(body, "gap-log-line") {
		t.Fatalf("gap log event published during backfill must be delivered; body: %s", body)
	}
	if !strings.Contains(body, "live-line") {
		t.Fatalf("expected live-line in output; body: %s", body)
	}
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
// persisted to the DB and published to the hub during backfill is emitted once.
func TestGetJobLogsHandler_OverlapDedupDuringBackfill(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	overlapLine := "overlap-persisted-and-hub"
	objKey := "logs/overlap-job.gz"

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}
	st.listLogsByRun.val = []store.Log{
		{ID: 1, RunID: runID, JobID: &jobID, ObjectKey: &objKey},
	}

	bs := bsmock.New()
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

	time.Sleep(50 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: "",
		Stream:    "stdout",
		Line:      overlapLine,
		JobID:     jobID,
	})

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
	if count := strings.Count(body, overlapLine); count != 1 {
		t.Fatalf("overlap line should appear exactly once, got %d; body: %s", count, body)
	}
	if !strings.Contains(body, "unique-live-line") {
		t.Fatalf("expected unique-live-line in output; body: %s", body)
	}
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
// whose content matches backfill but has a different timestamp is not deduped.
func TestGetJobLogsHandler_RepeatedLiveLineNotDropped(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	repeatedLine := "repeated-content"
	objKey := "logs/repeated-job.gz"

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusRunning}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusRunning}
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
	if count := strings.Count(body, repeatedLine); count != 2 {
		t.Fatalf("repeated line should appear exactly twice (backfill + live), got %d; body: %s", count, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event; body: %s", body)
	}
}
