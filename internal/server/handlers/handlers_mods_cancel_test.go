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
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusRunning, StartedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Second * 5), Valid: true}},
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

	// Ticket should be observed before the terminal done event to ensure
	// followers see the terminal state before the stream closes.
	ticketIdx := -1
	doneIdx := -1
	foundTicket := false
	foundDone := false
	for i, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicket = true
			if ticketIdx < 0 {
				ticketIdx = i
			}
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
			if doneIdx < 0 {
				doneIdx = i
			}
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
	if !(ticketIdx >= 0 && doneIdx > ticketIdx) {
		t.Fatalf("expected ticket to precede done (ticketIdx=%d, doneIdx=%d)", ticketIdx, doneIdx)
	}
}

// TestCancelTicket_OnlyPendingRunningStagesUpdated ensures only pending|running stages
// are transitioned to canceled and terminal stages are left untouched.
func TestCancelTicket_OnlyPendingRunningStagesUpdated(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	stgPending := store.Stage{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusPending}
	stgRunning := store.Stage{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusRunning, StartedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Second), Valid: true}}
	stgSucceeded := store.Stage{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusSucceeded}
	stgFailed := store.Stage{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusFailed}
	stgCanceled := store.Stage{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.StageStatusCanceled}

	st := &mockStore{
		getRunResult: store.Run{ID: pgtype.UUID{Bytes: id, Valid: true}, Status: store.RunStatusRunning},
		listStagesByRunResult: []store.Stage{
			stgPending, stgRunning, stgSucceeded, stgFailed, stgCanceled,
		},
	}
	handler := cancelTicketHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if !st.updateRunStatusCalled {
		t.Fatalf("expected UpdateRunStatus to be called")
	}
	// Only pending and running stages should be updated.
	if len(st.updateStageStatusCalls) != 2 {
		t.Fatalf("expected 2 stage updates, got %d", len(st.updateStageStatusCalls))
	}
	updated := map[string]bool{}
	for _, c := range st.updateStageStatusCalls {
		updated[uuid.UUID(c.ID.Bytes).String()] = true
		if c.Status != store.StageStatusCanceled {
			t.Fatalf("expected stage status canceled, got %s", c.Status)
		}
	}
	if !updated[uuid.UUID(stgPending.ID.Bytes).String()] || !updated[uuid.UUID(stgRunning.ID.Bytes).String()] {
		t.Fatalf("expected pending and running stages to be updated; got %+v", updated)
	}
	if updated[uuid.UUID(stgSucceeded.ID.Bytes).String()] || updated[uuid.UUID(stgFailed.ID.Bytes).String()] || updated[uuid.UUID(stgCanceled.ID.Bytes).String()] {
		t.Fatalf("did not expect terminal stages to be updated; got %+v", updated)
	}
}
