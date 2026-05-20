package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// TestGetJobLogsHandler_TerminalBackfillIncludesRetention verifies that terminal
// backfill includes retention frames already published to the hub.
func TestGetJobLogsHandler_TerminalBackfillIncludesRetention(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	objKey := "logs/terminal-job.gz"

	st := &jobStore{}
	st.getJob.val = store.Job{ID: jobID, RunID: runID, Status: domaintypes.JobStatusSuccess}
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
