package handlers

import (
	"context"
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
