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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestCancelRun_Success transitions a non-terminal run to canceled and updates jobs.
func TestCancelRun_Success(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        id.String(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: uuid.New().String(), Status: store.JobStatusRunning, StartedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Second * 5), Valid: true}},
		},
	}

	handler := cancelRunHandler(st, nil)

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
	if st.updateRunStatusParams.Status != store.RunStatusCancelled {
		t.Fatalf("expected status canceled, got %s", st.updateRunStatusParams.Status)
	}
	if !st.updateJobStatusCalled {
		t.Fatal("expected UpdateJobStatus to be called for jobs")
	}
}

// TestCancelRun_Idempotent verifies 200 when already terminal.
func TestCancelRun_Idempotent(t *testing.T) {
	tests := []struct {
		name      string
		runStatus store.RunStatus
	}{
		{
			name:      "already canceled",
			runStatus: store.RunStatusCancelled,
		},
		{
			name:      "already succeeded",
			runStatus: store.RunStatusFinished,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			st := &mockStore{getRunResult: store.Run{ID: id.String(), Status: tt.runStatus}}
			handler := cancelRunHandler(st, nil)
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

// TestCancelRun_BadID_And_NotFound verifies rejection of invalid IDs and not found handling.
// Run IDs are now KSUID strings; only empty/whitespace IDs are rejected.
func TestCancelRun_BadID_And_NotFound(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		urlID      string
		mockStore  *mockStore
		wantStatus int
		wantBody   string
	}{
		// Note: "invalid uuid format" test removed - with KSUID string IDs, any non-empty string is valid.
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
			name:       "run not found",
			id:         uuid.New().String(),
			urlID:      "",
			mockStore:  &mockStore{getRunErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := cancelRunHandler(tt.mockStore, nil)
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

// TestCancelRun_SSEPublish verifies that the handler publishes run and status events.
func TestCancelRun_SSEPublish(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        id.String(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
		listJobsByRunResult: []store.Job{},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	handler := cancelRunHandler(st, eventsService)

	body, _ := json.Marshal(map[string]string{"reason": "user requested"})
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", bytes.NewReader(body))
	req.SetPathValue("id", id.String())
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify that run and done status events were published via SSE.
	snapshot := eventsService.Hub().Snapshot(id.String())
	if len(snapshot) < 2 {
		t.Fatalf("expected at least 2 events (run + done status), got %d", len(snapshot))
	}

	// Run event should be observed before the terminal done event to ensure
	// followers see the terminal state before the stream closes.
	runIdx := -1
	doneIdx := -1
	foundRun := false
	foundDone := false
	for i, evt := range snapshot {
		if evt.Type == "run" {
			foundRun = true
			if runIdx < 0 {
				runIdx = i
			}
			// Verify run state is canceled and reason is present.
			var runData map[string]interface{}
			if err := json.Unmarshal(evt.Data, &runData); err != nil {
				t.Fatalf("failed to unmarshal run data: %v", err)
			}
			if state, ok := runData["state"].(string); !ok || state != "cancelled" {
				t.Fatalf("expected run state 'cancelled', got %v", runData["state"])
			}
			if metadata, ok := runData["metadata"].(map[string]interface{}); ok {
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

	if !foundRun {
		t.Fatal("expected run event in snapshot")
	}
	if !foundDone {
		t.Fatal("expected done status event in snapshot")
	}
	if runIdx < 0 || doneIdx <= runIdx {
		t.Fatalf("expected run to precede done (runIdx=%d, doneIdx=%d)", runIdx, doneIdx)
	}
}

// TestCancelRun_OnlyPendingRunningStagesUpdated ensures only pending|running jobs
// are transitioned to canceled and terminal jobs are left untouched.
func TestCancelRun_OnlyPendingRunningStagesUpdated(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	pendingID := uuid.New().String()
	runningID := uuid.New().String()
	succeededID := uuid.New().String()
	failedID := uuid.New().String()
	canceledID := uuid.New().String()

	stgPending := store.Job{ID: pendingID, Status: store.JobStatusCreated}
	stgRunning := store.Job{ID: runningID, Status: store.JobStatusRunning, StartedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Second), Valid: true}}
	stgSucceeded := store.Job{ID: succeededID, Status: store.JobStatusSuccess}
	stgFailed := store.Job{ID: failedID, Status: store.JobStatusFail}
	stgCanceled := store.Job{ID: canceledID, Status: store.JobStatusCancelled}

	st := &mockStore{
		getRunResult: store.Run{ID: id.String(), Status: store.RunStatusStarted},
		listJobsByRunResult: []store.Job{
			stgPending, stgRunning, stgSucceeded, stgFailed, stgCanceled,
		},
	}
	handler := cancelRunHandler(st, nil)

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
	// Only pending and running jobs should be updated.
	if len(st.updateJobStatusCalls) != 2 {
		t.Fatalf("expected 2 job updates, got %d", len(st.updateJobStatusCalls))
	}
	updated := map[string]bool{}
	for _, c := range st.updateJobStatusCalls {
		updated[c.ID] = true
		if c.Status != store.JobStatusCancelled {
			t.Fatalf("expected job status canceled, got %s", c.Status)
		}
	}
	if !updated[pendingID] || !updated[runningID] {
		t.Fatalf("expected pending and running jobs to be updated; got %+v", updated)
	}
	if updated[succeededID] || updated[failedID] || updated[canceledID] {
		t.Fatalf("did not expect terminal jobs to be updated; got %+v", updated)
	}
}

// TestCancelRun_NoStages verifies behavior when run has no jobs.
func TestCancelRun_NoStages(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:        id.String(),
			Status:    store.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		},
		listJobsByRunResult: []store.Job{},
	}
	handler := cancelRunHandler(st, nil)

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
	if st.updateJobStatusCalled {
		t.Fatal("did not expect UpdateJobStatus to be called when no jobs")
	}
}

// TestCancelRun_JSONBodyVariations tests different request body formats.
func TestCancelRun_JSONBodyVariations(t *testing.T) {
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
					ID:        id.String(),
					Status:    store.RunStatusStarted,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
				listJobsByRunResult: []store.Job{},
			}
			handler := cancelRunHandler(st, nil)

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

// TestCancelRun_StageDuration verifies duration calculation for jobs.
func TestCancelRun_StageDuration(t *testing.T) {
	tests := []struct {
		name            string
		startedAt       pgtype.Timestamptz
		wantDurationSet bool
	}{
		{
			name:            "running job with started time",
			startedAt:       pgtype.Timestamptz{Time: time.Now().Add(-5 * time.Second), Valid: true},
			wantDurationSet: true,
		},
		{
			name:            "pending job without started time",
			startedAt:       pgtype.Timestamptz{Valid: false},
			wantDurationSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := domaintypes.NewRunID()
			jobID := domaintypes.NewJobID()
			st := &mockStore{
				getRunResult: store.Run{
					ID:        id.String(),
					Status:    store.RunStatusStarted,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
				listJobsByRunResult: []store.Job{
					{
						ID:        jobID.String(),
						Status:    store.JobStatusRunning,
						StartedAt: tt.startedAt,
					},
				},
			}
			handler := cancelRunHandler(st, nil)

			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/cancel", nil)
			req.SetPathValue("id", id.String())
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusAccepted {
				t.Fatalf("expected 202, got %d", rr.Code)
			}
			if len(st.updateJobStatusCalls) != 1 {
				t.Fatalf("expected 1 job update, got %d", len(st.updateJobStatusCalls))
			}
			call := st.updateJobStatusCalls[0]
			if tt.wantDurationSet && call.DurationMs == 0 {
				t.Fatal("expected duration to be set for started job")
			}
			if !tt.wantDurationSet && call.DurationMs != 0 {
				t.Fatalf("expected duration to be 0 for pending job, got %d", call.DurationMs)
			}
		})
	}
}
