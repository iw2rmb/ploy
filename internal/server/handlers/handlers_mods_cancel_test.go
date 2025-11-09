package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestCancelTicket_Success transitions a non-terminal run to canceled and updates stages.
func TestCancelTicket_Success(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: id, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://example/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
		listStagesByRunResult: []store.Stage{
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, StartedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Second * 5), Valid: true}},
		},
	}

	handler := cancelTicketHandler(st, nil)

	body, _ := json.Marshal(map[string]string{"reason": "user requested"})
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", bytes.NewReader(body))
	req.SetPathValue("id", id.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.Status != store.RunStatusCanceled {
		t.Fatalf("expected status canceled, got %s", st.updateRunStatusParams.Status)
	}
	if !st.updateStageStatusCalled {
		t.Fatal("expected UpdateStageStatus to be called for stages")
	}
}

// TestCancelTicket_Idempotent verifies 200 when already terminal.
func TestCancelTicket_Idempotent(t *testing.T) {
	id := uuid.New()
	st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: id, Valid: true}, Status: store.RunStatusCanceled}}
	handler := cancelTicketHandler(st, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus on idempotent path")
	}
}

// TestCancelTicket_BadID and NotFound paths.
func TestCancelTicket_BadID_And_NotFound(t *testing.T) {
	// Bad id
	st1 := &mockStore{}
	h1 := cancelTicketHandler(st1, nil)
	r1 := httptest.NewRequest(http.MethodPost, "/v1/mods/abc/cancel", nil)
	r1.SetPathValue("id", "abc")
	rr1 := httptest.NewRecorder()
	h1.ServeHTTP(rr1, r1)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad id, got %d", rr1.Code)
	}

	// Not found
	id := uuid.New()
	st2 := &mockStore{getRunErr: pgx.ErrNoRows}
	h2 := cancelTicketHandler(st2, nil)
	r2 := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", nil)
	r2.SetPathValue("id", id.String())
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, r2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing ticket, got %d", rr2.Code)
	}
}

// TestCancelTicket_SSEPublish verifies that the handler publishes ticket and status events.
func TestCancelTicket_SSEPublish(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: id, Valid: true},
			Status:    store.RunStatusRunning,
			RepoUrl:   "https://example/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
		listStagesByRunResult: []store.Stage{},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	handler := cancelTicketHandler(st, eventsService)

	body, _ := json.Marshal(map[string]string{"reason": "user requested"})
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", bytes.NewReader(body))
	req.SetPathValue("id", id.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify that ticket and done status events were published via SSE.
	snapshot := eventsService.Hub().Snapshot(id.String())
	if len(snapshot) < 2 {
		t.Fatalf("expected at least 2 events (ticket + done status), got %d", len(snapshot))
	}

	// First event should be the ticket event.
	foundTicket := false
	foundDone := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicket = true
			// Verify ticket state is canceled and reason is present.
			var ticketData map[string]interface{}
			if err := json.Unmarshal(evt.Data, &ticketData); err != nil {
				t.Fatalf("failed to unmarshal ticket data: %v", err)
			}
			if state, ok := ticketData["state"].(string); !ok || state != "cancelled" {
				t.Fatalf("expected ticket state 'cancelled', got %v", ticketData["state"])
			}
			if metadata, ok := ticketData["metadata"].(map[string]interface{}); ok {
				if reason, ok := metadata["reason"].(string); !ok || reason != "user requested" {
					t.Fatalf("expected reason 'user requested', got %v", reason)
				}
			} else {
				t.Fatal("expected metadata with reason")
			}
		}
		if evt.Type == "done" {
			foundDone = true
			// Verify done status.
			var statusData map[string]interface{}
			if err := json.Unmarshal(evt.Data, &statusData); err != nil {
				t.Fatalf("failed to unmarshal status data: %v", err)
			}
			if status, ok := statusData["status"].(string); !ok || status != "done" {
				t.Fatalf("expected status 'done', got %v", statusData["status"])
			}
		}
	}

	if !foundTicket {
		t.Fatal("expected ticket event in snapshot")
	}
	if !foundDone {
		t.Fatal("expected done status event in snapshot")
	}
}
