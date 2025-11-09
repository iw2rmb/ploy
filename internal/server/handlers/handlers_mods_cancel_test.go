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
	tests := []struct {
		name      string
		runStatus store.RunStatus
	}{
		{
			name:      "already canceled",
			runStatus: store.RunStatusCanceled,
		},
		{
			name:      "already succeeded",
			runStatus: store.RunStatusSucceeded,
		},
		{
			name:      "already failed",
			runStatus: store.RunStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: id, Valid: true}, Status: tt.runStatus}}
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
		})
	}
}

// TestCancelTicket_BadID and NotFound paths.
func TestCancelTicket_BadID_And_NotFound(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		urlID      string
		mockStore  *mockStore
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid uuid format",
			id:         "abc",
			urlID:      "abc",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid id: invalid uuid",
		},
		{
			name:       "empty id",
			id:         "",
			urlID:      "placeholder",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "id path parameter is required",
		},
		{
			name:       "whitespace only id",
			id:         "   ",
			urlID:      "placeholder",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "id path parameter is required",
		},
		{
			name:       "ticket not found",
			id:         uuid.New().String(),
			urlID:      "",
			mockStore:  &mockStore{getRunErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "ticket not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := cancelTicketHandler(tt.mockStore, nil)
			urlID := tt.urlID
			if urlID == "" {
				urlID = tt.id
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+urlID+"/cancel", nil)
			req.SetPathValue("id", tt.id)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if tt.wantBody != "" && !bytes.Contains(rr.Body.Bytes(), []byte(tt.wantBody)) {
				t.Fatalf("expected body to contain %q, got %q", tt.wantBody, rr.Body.String())
			}
		})
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

// TestCancelTicket_NoStages verifies behavior when run has no stages.
func TestCancelTicket_NoStages(t *testing.T) {
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
	handler := cancelTicketHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateStageStatusCalled {
		t.Fatal("did not expect UpdateStageStatus to be called when no stages")
	}
}

// TestCancelTicket_JSONBodyVariations tests different request body formats.
func TestCancelTicket_JSONBodyVariations(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "empty body",
			body:       "",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "empty json object",
			body:       "{}",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "reason provided",
			body:       `{"reason": "test cancellation"}`,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "reason null",
			body:       `{"reason": null}`,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "reason empty string",
			body:       `{"reason": ""}`,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "malformed json ignored",
			body:       `{bad json`,
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			st := &mockStore{
				getRunResult: store.Run{
					ID:        pgtype.UUID{Bytes: id, Valid: true},
					Status:    store.RunStatusRunning,
					RepoUrl:   "https://example/repo.git",
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
				listStagesByRunResult: []store.Stage{},
			}
			handler := cancelTicketHandler(st, nil)

			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", bytes.NewReader([]byte(tt.body)))
			req.SetPathValue("id", id.String())
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestCancelTicket_StageDuration verifies duration calculation for stages.
func TestCancelTicket_StageDuration(t *testing.T) {
	tests := []struct {
		name            string
		startedAt       pgtype.Timestamptz
		wantDurationSet bool
	}{
		{
			name:            "running stage with started time",
			startedAt:       pgtype.Timestamptz{Time: time.Now().Add(-5 * time.Second), Valid: true},
			wantDurationSet: true,
		},
		{
			name:            "pending stage without started time",
			startedAt:       pgtype.Timestamptz{Valid: false},
			wantDurationSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			stageID := uuid.New()
			st := &mockStore{
				getRunResult: store.Run{
					ID:        pgtype.UUID{Bytes: id, Valid: true},
					Status:    store.RunStatusRunning,
					RepoUrl:   "https://example/repo.git",
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
				listStagesByRunResult: []store.Stage{
					{
						ID:        pgtype.UUID{Bytes: stageID, Valid: true},
						Status:    store.StageStatusRunning,
						StartedAt: tt.startedAt,
					},
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
			if len(st.updateStageStatusCalls) != 1 {
				t.Fatalf("expected 1 stage update, got %d", len(st.updateStageStatusCalls))
			}
			call := st.updateStageStatusCalls[0]
			if tt.wantDurationSet && call.DurationMs == 0 {
				t.Fatal("expected duration to be set for started stage")
			}
			if !tt.wantDurationSet && call.DurationMs != 0 {
				t.Fatalf("expected duration to be 0 for pending stage, got %d", call.DurationMs)
			}
		})
	}
}
