package handlers

import (
	"context"
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

// TestListRunsHandler verifies the GET /v1/runs handler with various scenarios.
func TestListRunsHandler(t *testing.T) {
	t.Parallel()

	// Sample run for testing.
	sampleRunID := uuid.New()
	sampleRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name           string
		query          string
		mockRuns       []store.Run
		mockRunsErr    error
		mockRepoCounts []store.CountRunReposByStatusRow
		wantStatus     int
		wantRunCount   int
	}{
		{
			name:         "empty list",
			query:        "",
			mockRuns:     []store.Run{},
			wantStatus:   http.StatusOK,
			wantRunCount: 0,
		},
		{
			name:         "single run without repos",
			query:        "",
			mockRuns:     []store.Run{sampleRun},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:     "single run with repo counts",
			query:    "",
			mockRuns: []store.Run{sampleRun},
			mockRepoCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 2},
				{Status: store.RunRepoStatusSucceeded, Count: 1},
			},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:         "pagination with limit",
			query:        "?limit=10",
			mockRuns:     []store.Run{sampleRun},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:         "pagination with offset",
			query:        "?limit=10&offset=5",
			mockRuns:     []store.Run{},
			wantStatus:   http.StatusOK,
			wantRunCount: 0,
		},
		{
			name:       "invalid limit",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid offset",
			query:      "?offset=-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "database error",
			query:       "",
			mockRunsErr: pgx.ErrTxClosed,
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup mock store.
			m := &mockStore{
				listRunsResult:              tc.mockRuns,
				listRunsErr:                 tc.mockRunsErr,
				countRunReposByStatusResult: tc.mockRepoCounts,
			}

			// Create handler and request.
			handler := listRunsHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs"+tc.query, nil)
			w := httptest.NewRecorder()

			// Execute handler.
			handler(w, req)

			// Check status code.
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}

			// For successful responses, verify run count.
			if tc.wantStatus == http.StatusOK {
				var resp struct {
					Runs []RunBatchSummary `json:"runs"`
				}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(resp.Runs) != tc.wantRunCount {
					t.Errorf("run count = %d, want %d", len(resp.Runs), tc.wantRunCount)
				}

				// Verify repo counts are populated when available.
				if len(tc.mockRepoCounts) > 0 && len(resp.Runs) > 0 {
					if resp.Runs[0].Counts == nil {
						t.Error("expected repo counts to be populated")
					} else if resp.Runs[0].Counts.Total != 3 {
						t.Errorf("total count = %d, want 3", resp.Runs[0].Counts.Total)
					}
				}
			}
		})
	}
}

// TestGetRunHandler verifies the GET /v1/runs/{id} handler.
func TestGetRunHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	sampleRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name           string
		runID          string
		mockRun        store.Run
		mockRunErr     error
		mockRepoCounts []store.CountRunReposByStatusRow
		wantStatus     int
	}{
		{
			name:       "valid run",
			runID:      sampleRunID.String(),
			mockRun:    sampleRun,
			wantStatus: http.StatusOK,
		},
		{
			name:    "run with repo counts",
			runID:   sampleRunID.String(),
			mockRun: sampleRun,
			mockRepoCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusRunning, Count: 1},
				{Status: store.RunRepoStatusPending, Count: 2},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty id",
			runID:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid uuid",
			runID:      "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "run not found",
			runID:      uuid.New().String(),
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "database error",
			runID:      sampleRunID.String(),
			mockRunErr: pgx.ErrTxClosed,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:                tc.mockRun,
				getRunErr:                   tc.mockRunErr,
				countRunReposByStatusResult: tc.mockRepoCounts,
			}

			handler := getRunHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tc.runID, nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// Verify response structure for successful requests.
			if tc.wantStatus == http.StatusOK {
				var resp RunBatchSummary
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if resp.ID != tc.runID {
					t.Errorf("id = %s, want %s", resp.ID, tc.runID)
				}
				if resp.Status != tc.mockRun.Status {
					t.Errorf("status = %s, want %s", resp.Status, tc.mockRun.Status)
				}

				// Check repo counts if expected.
				if len(tc.mockRepoCounts) > 0 {
					if resp.Counts == nil {
						t.Error("expected repo counts to be populated")
					} else if resp.Counts.Total != 3 {
						t.Errorf("total count = %d, want 3", resp.Counts.Total)
					}
				}
			}
		})
	}
}

// TestStopRunHandler verifies the POST /v1/runs/{id}/stop handler.
func TestStopRunHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	pendingRepoID := uuid.New()
	runningRepoID := uuid.New()

	runningRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:         pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Name:       ptrString("test-batch"),
		RepoUrl:    "https://github.com/example/repo.git",
		Status:     store.RunStatusCanceled,
		BaseRef:    "main",
		TargetRef:  "feature",
		CreatedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		FinishedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runID            string
		mockRun          store.Run
		mockRunErr       error
		mockRepos        []store.RunRepo
		updateStatusErr  error
		wantStatus       int
		wantCanceledRun  bool
		wantReposUpdated int
	}{
		{
			name:    "stop running run",
			runID:   sampleRunID.String(),
			mockRun: runningRun,
			mockRepos: []store.RunRepo{
				{
					ID:     pgtype.UUID{Bytes: pendingRepoID, Valid: true},
					RunID:  pgtype.UUID{Bytes: sampleRunID, Valid: true},
					Status: store.RunRepoStatusPending,
				},
				{
					ID:     pgtype.UUID{Bytes: runningRepoID, Valid: true},
					RunID:  pgtype.UUID{Bytes: sampleRunID, Valid: true},
					Status: store.RunRepoStatusRunning,
				},
			},
			wantStatus:       http.StatusOK,
			wantCanceledRun:  true,
			wantReposUpdated: 1, // Only pending repo is updated
		},
		{
			name:            "stop already canceled run (idempotent)",
			runID:           sampleRunID.String(),
			mockRun:         canceledRun,
			wantStatus:      http.StatusOK,
			wantCanceledRun: false, // Already canceled, no update
		},
		{
			name:       "empty id",
			runID:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid uuid",
			runID:      "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "run not found",
			runID:      uuid.New().String(),
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:            "update status error",
			runID:           sampleRunID.String(),
			mockRun:         runningRun,
			updateStatusErr: pgx.ErrTxClosed,
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:            tc.mockRun,
				getRunErr:               tc.mockRunErr,
				updateRunStatusErr:      tc.updateStatusErr,
				listRunReposByRunResult: tc.mockRepos,
			}

			handler := stopRunHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/stop", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// Verify run status was updated.
			if tc.wantCanceledRun {
				if !m.updateRunStatusCalled {
					t.Error("expected UpdateRunStatus to be called")
				}
				if m.updateRunStatusParams.Status != store.RunStatusCanceled {
					t.Errorf("update status = %s, want %s", m.updateRunStatusParams.Status, store.RunStatusCanceled)
				}
			}

			// Verify pending repos were updated.
			if tc.wantReposUpdated > 0 {
				if len(m.updateRunRepoStatusParams) != tc.wantReposUpdated {
					t.Errorf("updated repos = %d, want %d", len(m.updateRunRepoStatusParams), tc.wantReposUpdated)
				}
				for _, params := range m.updateRunRepoStatusParams {
					if params.Status != store.RunRepoStatusCancelled {
						t.Errorf("repo status = %s, want cancelled", params.Status)
					}
				}
			}
		})
	}
}

// TestIsTerminalRunStatus verifies the helper function.
func TestIsTerminalRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status store.RunStatus
		want   bool
	}{
		{store.RunStatusQueued, false},
		{store.RunStatusAssigned, false},
		{store.RunStatusRunning, false},
		{store.RunStatusSucceeded, true},
		{store.RunStatusFailed, true},
		{store.RunStatusCanceled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := isTerminalRunStatus(tc.status); got != tc.want {
				t.Errorf("isTerminalRunStatus(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestRunRepoCounts verifies the getRunRepoCounts helper.
func TestGetRunRepoCounts(t *testing.T) {
	t.Parallel()

	runID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	tests := []struct {
		name       string
		mockCounts []store.CountRunReposByStatusRow
		mockErr    error
		wantTotal  int32
		wantErr    bool
	}{
		{
			name: "all statuses",
			mockCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 1},
				{Status: store.RunRepoStatusRunning, Count: 2},
				{Status: store.RunRepoStatusSucceeded, Count: 3},
				{Status: store.RunRepoStatusFailed, Count: 4},
				{Status: store.RunRepoStatusSkipped, Count: 5},
				{Status: store.RunRepoStatusCancelled, Count: 6},
			},
			wantTotal: 21,
		},
		{
			name:       "empty",
			mockCounts: []store.CountRunReposByStatusRow{},
			wantTotal:  0,
		},
		{
			name:    "error",
			mockErr: pgx.ErrTxClosed,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				countRunReposByStatusResult: tc.mockCounts,
				countRunReposByStatusErr:    tc.mockErr,
			}

			counts, err := getRunRepoCounts(context.Background(), m, runID)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if counts.Total != tc.wantTotal {
				t.Errorf("total = %d, want %d", counts.Total, tc.wantTotal)
			}
		})
	}
}

// ptrString returns a pointer to the given string.
func ptrString(s string) *string {
	return &s
}
