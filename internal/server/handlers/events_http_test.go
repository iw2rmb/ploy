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
	st := func() *runStore { st := &runStore{}; st.getRun.err = pgx.ErrNoRows; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	runID := testRunIDKSUID
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestGetRunLogsHandler_InvalidRunID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &runStore{}
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/invalid/logs", nil)
	req.SetPathValue("id", "invalid")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusBadRequest)
	if st.getRun.called {
		t.Fatal("expected GetRun not to be called")
	}
}

func TestGetRunLogsHandler_MissingID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &runStore{}
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//logs", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestGetRunLogsHandler_DatabaseError(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := func() *runStore { st := &runStore{}; st.getRun.err = pgx.ErrTxClosed; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	runID := testRunIDKSUID
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusInternalServerError)
	if !st.getRun.called {
		t.Fatal("expected GetRun to be called")
	}
}

// TestGetRunLogsHandler_LifecycleOnly verifies that the run SSE stream
// emits only lifecycle events (stage, done) and not container log frames.
func TestGetRunLogsHandler_LifecycleOnly(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	runID := testRunIDKSUID
	st := func() *runStore { st := &runStore{}; st.getRun.val = store.Run{ID: domaintypes.RunID(runID)}; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish.
	time.Sleep(25 * time.Millisecond)
	ctx := context.Background()
	// Publish a stage event (lifecycle) then done.
	_ = hub.PublishStage(ctx, domaintypes.RunID(runID), logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "info",
		Line:      "step started",
	})
	_ = hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stream to finish")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: stage") {
		t.Fatalf("expected stage event in body: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event in body: %s", body)
	}
	// Container log events must NOT appear on the run stream.
	if strings.Contains(body, "event: log") {
		t.Fatalf("unexpected log event on run stream: %s", body)
	}
	if strings.Contains(body, "event: retention") {
		t.Fatalf("unexpected retention event on run stream: %s", body)
	}
}

// TestGetRunLogsHandler_RejectsNonLifecycleFrames verifies that log and retention
// frames published to the run stream are filtered out by the lifecycle filter.
func TestGetRunLogsHandler_RejectsNonLifecycleFrames(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	runID := testRunIDKSUID
	st := func() *runStore { st := &runStore{}; st.getRun.val = store.Run{ID: domaintypes.RunID(runID)}; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	ctx := context.Background()

	// Publish log and retention frames that should be rejected.
	_ = hub.PublishLog(ctx, domaintypes.RunID(runID), logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "container output",
	})
	_ = hub.PublishRetention(ctx, domaintypes.RunID(runID), logstream.RetentionHint{
		Retained: true,
	})
	// Publish a lifecycle stage and done to terminate the stream.
	_ = hub.PublishStage(ctx, domaintypes.RunID(runID), logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "info",
		Line:      "step ok",
	})
	_ = hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stream to finish")
	}

	body := rr.Body.String()
	if strings.Contains(body, "event: log") {
		t.Fatalf("unexpected log event on run stream: %s", body)
	}
	if strings.Contains(body, "event: retention") {
		t.Fatalf("unexpected retention event on run stream: %s", body)
	}
	if !strings.Contains(body, "event: stage") {
		t.Fatalf("expected stage event in body: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event in body: %s", body)
	}
}

func TestGetRunLogsHandler_Resume(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	runID := testRunIDKSUID
	st := func() *runStore { st := &runStore{}; st.getRun.val = store.Run{ID: domaintypes.RunID(runID)}; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	// Pre-publish a stage event so history contains id=1 before subscriber joins.
	ctx := context.Background()
	_ = hub.PublishStage(ctx, domaintypes.RunID(runID), logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "info",
		Line:      "first",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	req.Header.Set("Last-Event-ID", "1")
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish, then publish id=2 and done.
	time.Sleep(25 * time.Millisecond)
	_ = hub.PublishStage(ctx, domaintypes.RunID(runID), logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "info",
		Line:      "second",
	})
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

// TestGetRunLogsHandler_TerminalRunDone verifies that a fresh connection
// to a terminal run receives a done sentinel immediately.
func TestGetRunLogsHandler_TerminalRunDone(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	runID := testRunIDKSUID
	st := func() *runStore {
		st := &runStore{}
		st.getRun.val = store.Run{ID: domaintypes.RunID(runID), Status: domaintypes.RunStatusFinished}
		return st
	}()
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected done event for terminal run: %s", body)
	}

	// Verify done payload contains the run status.
	var found bool
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "Finished") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected done payload with Finished status: %s", body)
	}
}

// TestGetRunLogsHandler_StageEventPayload verifies that stage events preserve
// their payload structure when streamed to SSE clients.
func TestGetRunLogsHandler_StageEventPayload(t *testing.T) {
	t.Parallel()

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	hub := eventsService.Hub()

	runID := testRunIDKSUID
	st := func() *runStore { st := &runStore{}; st.getRun.val = store.Run{ID: domaintypes.RunID(runID)}; return st }()
	h := getRunLogsHandler(st, nil, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/logs", nil)
	req.SetPathValue("id", runID)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)

	ctx := context.Background()
	stageRec := logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "info",
		Line:      "gate check passed",
	}
	if err := hub.PublishStage(ctx, domaintypes.RunID(runID), stageRec); err != nil {
		t.Fatalf("publish stage: %v", err)
	}
	if err := hub.PublishStatus(ctx, domaintypes.RunID(runID), logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stream to finish")
	}

	body := rr.Body.String()

	// Extract the first "data: " line with JSON.
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

	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonLine), &payload); err != nil {
		t.Fatalf("unmarshal stage payload: %v", err)
	}

	if got := payload["stream"]; got != "info" {
		t.Errorf("stream = %v, want %q", got, "info")
	}
	if got := payload["line"]; got != "gate check passed" {
		t.Errorf("line = %v, want %q", got, "gate check passed")
	}
}
