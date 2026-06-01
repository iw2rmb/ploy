package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const testRunSHASeed = "0123456789abcdef0123456789abcdef01234567"

func TestCancelRunHandlerV1_CancelsRunAndWork(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &runStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !st.cancelRun.called {
		t.Fatalf("expected CancelRun to be called")
	}
	if st.cancelRun.params != runID.String() {
		t.Fatalf("expected CancelRun run id %q, got %q", runID, st.cancelRun.params)
	}
}

func TestCancelRunHandlerV1_CancelRunError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &runStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}
	st.cancelRun.err = errors.New("db exploded")

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
	if !st.cancelRun.called {
		t.Fatalf("expected CancelRun to be called")
	}
}

func TestCancelRunHandlerV1_TerminalRunIsIdempotent(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	st := &runStore{}
	st.getRun.val = store.Run{
		ID:        runID,
		MigID:     domaintypes.NewMigID(),
		SpecID:    domaintypes.NewSpecID(),
		Status:    domaintypes.RunStatusCancelled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/cancel", nil)
	req.SetPathValue("run_id", runID.String())
	rr := httptest.NewRecorder()

	cancelRunHandlerV1(st).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if st.cancelRun.called {
		t.Fatalf("did not expect CancelRun to be called for terminal run")
	}
}

func TestRestartRunHandler(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	tests := []struct {
		name       string
		setup      func(*runStore)
		wantStatus int
		verify     func(*testing.T, *runStore)
	}{
		{
			name: "success",
			setup: func(st *runStore) {
				st.restartRun.val = store.Run{
					ID:        runID,
					MigID:     domaintypes.NewMigID(),
					SpecID:    domaintypes.NewSpecID(),
					Status:    domaintypes.RunStatusQueued,
					Attempt:   2,
					CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				}
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *runStore) {
				t.Helper()
				if !st.restartRun.called {
					t.Fatal("expected RestartRun to be called")
				}
				if st.restartRun.params != runID.String() {
					t.Fatalf("RestartRun id=%q, want %q", st.restartRun.params, runID)
				}
			},
		},
		{
			name: "active run conflict",
			setup: func(st *runStore) {
				st.restartRun.err = store.ErrRunRestartActive
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "cancelled wave conflict",
			setup: func(st *runStore) {
				st.restartRun.err = store.ErrRunRestartWaveCancelled
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &runStore{}
			tt.setup(st)

			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID.String()+"/restart", nil)
			req.SetPathValue("run_id", runID.String())
			rr := httptest.NewRecorder()

			restartRunHandler(st).ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st)
			}
		})
	}
}
