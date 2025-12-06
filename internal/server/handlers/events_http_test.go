package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

func TestGetModEventsHandler_TicketNotFound(t *testing.T) {
	t.Parallel()
	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	st := &mockStore{getRunErr: pgx.ErrNoRows}
	h := getModEventsHandler(st, eventsService)

	ticketID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID+"/events", nil)
	req.SetPathValue("id", ticketID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetModEventsHandler_InvalidTicketID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{}
	h := getModEventsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/invalid/events", nil)
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

func TestGetModEventsHandler_MissingID(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{}
	h := getModEventsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods//events", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetModEventsHandler_DatabaseError(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	st := &mockStore{getRunErr: pgx.ErrTxClosed}
	h := getModEventsHandler(st, eventsService)

	ticketID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID+"/events", nil)
	req.SetPathValue("id", ticketID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
}

func TestGetModEventsHandler_Success(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	ticketID := uuid.New()
	st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: ticketID, Valid: true}}}
	h := getModEventsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String()+"/events", nil)
	req.SetPathValue("id", ticketID.String())
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish
	time.Sleep(25 * time.Millisecond)
	ctx := context.Background()
	_ = hub.PublishLog(ctx, ticketID.String(), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "hello"})
	_ = hub.PublishStatus(ctx, ticketID.String(), logstream.Status{Status: "completed"})

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

func TestGetModEventsHandler_Resume(t *testing.T) {
	t.Parallel()
	eventsService, _ := createTestEventsService()
	hub := eventsService.Hub()
	ticketID := uuid.New()
	st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: ticketID, Valid: true}}}
	h := getModEventsHandler(st, eventsService)

	// Pre-publish an event so history contains id=1 before subscriber joins.
	ctx := context.Background()
	_ = hub.PublishLog(ctx, ticketID.String(), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "first"})

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String()+"/events", nil)
	req.SetPathValue("id", ticketID.String())
	req.Header.Set("Last-Event-ID", "1")
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish, then publish id=2 and done
	time.Sleep(25 * time.Millisecond)
	_ = hub.PublishLog(ctx, ticketID.String(), logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339), Stream: "stdout", Line: "second"})
	_ = hub.PublishStatus(ctx, ticketID.String(), logstream.Status{Status: "completed"})

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

// TestGetModEventsHandler_EnrichedLogPayload verifies that the SSE HTTP
// handler preserves enriched log fields (node_id, job_id, mod_type,
// step_index) in the JSON payload streamed to clients. This ensures the
// HTTP layer does not strip or alter the enriched LogRecord shape used
// by CLI consumers (mods logs, runs follow).
func TestGetModEventsHandler_EnrichedLogPayload(t *testing.T) {
	t.Parallel()

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	hub := eventsService.Hub()

	ticketID := uuid.New()
	st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: ticketID, Valid: true}}}
	h := getModEventsHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID.String()+"/events", nil)
	req.SetPathValue("id", ticketID.String())
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	// Allow subscription to establish before publishing events.
	time.Sleep(25 * time.Millisecond)

	ctx := context.Background()
	enriched := logstream.LogRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "enriched line",
		NodeID:    "node-abc123",
		JobID:     "job-def456",
		ModType:   "mod",
		StepIndex: 2000,
	}
	if err := hub.PublishLog(ctx, ticketID.String(), enriched); err != nil {
		t.Fatalf("publish enriched log: %v", err)
	}
	if err := hub.PublishStatus(ctx, ticketID.String(), logstream.Status{Status: "completed"}); err != nil {
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

	if got := payload["node_id"]; got != "node-abc123" {
		t.Errorf("node_id = %v, want %q", got, "node-abc123")
	}
	if got := payload["job_id"]; got != "job-def456" {
		t.Errorf("job_id = %v, want %q", got, "job-def456")
	}
	if got := payload["mod_type"]; got != "mod" {
		t.Errorf("mod_type = %v, want %q", got, "mod")
	}
	if got := payload["step_index"]; got != float64(2000) {
		t.Errorf("step_index = %v, want %v", got, 2000)
	}
}
