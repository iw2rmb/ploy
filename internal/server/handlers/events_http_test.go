package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// testRunIDKSUID is a synthetic KSUID-like ID (27 characters) used for tests.
const testRunIDKSUID = "123456789012345678901234567"

func TestGetRunLogsHandler_TicketNotFound(t *testing.T) {
	t.Parallel()
	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	st := &mockStore{getRunErr: pgx.ErrNoRows}
	h := getRunLogsHandler(st, eventsService)

	runID := testRunIDKSUID
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetRunLogsHandler_InvalidRunID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{}
	h := getRunLogsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/invalid/logs", nil)
	req.SetPathValue("id", "invalid")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.getRunCalled {
		t.Fatal("expected GetRun not to be called")
	}
}

func TestGetRunLogsHandler_MissingID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{}
	h := getRunLogsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//logs", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetRunLogsHandler_DatabaseError(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{getRunErr: pgx.ErrTxClosed}
	h := getRunLogsHandler(st, eventsService)

	runID := testRunIDKSUID
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
}

func TestGetRunLogsHandler_Success(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	runID := testRunIDKSUID
	st := &mockStore{getRunResult: store.Run{ID: domaintypes.RunID(runID)}}
	h := getRunLogsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish
	time.Sleep(25 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishLog(ctx, domaintypes.RunID(runID), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "hello"})
	_ = hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stream to finish")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "event: done") {
		t.Fatalf("unexpected SSE body: %s", body)
	}
}

func TestGetRunLogsHandler_Resume(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	runID := testRunIDKSUID
	st := &mockStore{getRunResult: store.Run{ID: domaintypes.RunID(runID)}}
	h := getRunLogsHandler(st, eventsService)

	// Pre-publish an event so history contains id=1 before subscriber joins.
	ctx := context.Background()
	_ = hub.PublishLog(ctx, domaintypes.RunID(runID), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "first"})

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	req.Header.Set("Last-Event-ID", "1")
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish, then publish id=2 and done
	time.Sleep(25 * time.Millisecond)
	_ = hub.PublishLog(ctx, domaintypes.RunID(runID), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "second"})
	_ = hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for resumed stream")
	}

	body := rr.Body.String()
	if strings.Contains(body, "id: 1\n") {
		t.Fatalf("resume should not include id 1; body: %s", body)
	}
	if !strings.Contains(body, "id: 2\n") || !strings.Contains(body, "event: done") {
		t.Fatalf("resume body missing expected frames: %s", body)
	}
}

// TestGetRunLogsHandler_EnrichedLogPayload verifies that the SSE HTTP
// handler preserves enriched log fields (node_id, job_id, job_type,
// next_id) in the JSON payload streamed to clients. This ensures the
// HTTP layer does not strip or alter the enriched LogRecord shape used
// by CLI consumers (run logs).
func TestGetRunLogsHandler_EnrichedLogPayload(t *testing.T) {
	t.Parallel()

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	hub := eventsService.Hub()

	runID := testRunIDKSUID
	st := &mockStore{getRunResult: store.Run{ID: domaintypes.RunID(runID)}}
	h := getRunLogsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish before publishing events.
	time.Sleep(25 * time.Millisecond)

	ctx := context.Background()
	jobID := domaintypes.NewJobID()
	enriched := logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "enriched line",
		NodeID:    "aB3xY9",
		JobID:     jobID,
		JobType:   "mod",
	}
	if err := hub.PublishLog(ctx, domaintypes.RunID(runID), enriched); err != nil {
		t.Fatalf("publish enriched log: %v", err)
	}
	if err := hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for enriched stream to finish")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()

	// Extract the first "data: " line that contains a JSON object.
	var jsonLine string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "{") {
			jsonLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if jsonLine == "" {
		t.Fatalf("no JSON data line found in body: %s", body)
	}

	// Unmarshal and assert enriched fields are present with expected values.
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonLine), &payload); err != nil {
		t.Fatalf("unmarshal enriched log payload: %v", err)
	}

	if got := payload["node_id"]; got != "aB3xY9" {
		t.Errorf("node_id = %v, want %q", got, "aB3xY9")
	}
	if got := payload["job_id"]; got != jobID.String() {
		t.Errorf("job_id = %v, want %q", got, jobID.String())
	}
	if got := payload["job_type"]; got != "mod" {
		t.Errorf("job_type = %v, want %q", got, "mod")
	}
	if _, ok := payload["next_id"]; ok {
		t.Errorf("next_id should be absent, got %v", payload["next_id"])
	}
}
