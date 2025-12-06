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

// TestAddRunRepoHandler verifies the POST /v1/runs/{id}/repos handler.
func TestAddRunRepoHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	sampleRepoID := uuid.New()

	runningRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	createdRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/new-repo.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name          string
		runID         string
		body          string
		mockRun       store.Run
		mockRunErr    error
		mockRepoRes   store.RunRepo
		mockRepoErr   error
		wantStatus    int
		wantRepoID    string
		wantCallStore bool
	}{
		{
			name:          "valid add repo",
			runID:         sampleRunID.String(),
			body:          `{"repo_url":"https://github.com/example/new-repo.git","base_ref":"main","target_ref":"feature-2"}`,
			mockRun:       runningRun,
			mockRepoRes:   createdRepo,
			wantStatus:    http.StatusCreated,
			wantRepoID:    sampleRepoID.String(),
			wantCallStore: true,
		},
		{
			name:       "empty id",
			runID:      "",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid uuid",
			runID:      "not-a-uuid",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "run not found",
			runID:      uuid.New().String(),
			mockRunErr: pgx.ErrNoRows,
			body:       `{}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "run is terminal",
			runID:      sampleRunID.String(),
			body:       `{"repo_url":"https://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    canceledRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "missing repo_url",
			runID:      sampleRunID.String(),
			body:       `{"base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing base_ref",
			runID:      sampleRunID.String(),
			body:       `{"repo_url":"https://github.com/example/repo.git","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing target_ref",
			runID:      sampleRunID.String(),
			body:       `{"repo_url":"https://github.com/example/repo.git","base_ref":"main"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid repo_url scheme",
			runID:      sampleRunID.String(),
			body:       `{"repo_url":"ftp://example.com/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "store error",
			runID:       sampleRunID.String(),
			body:        `{"repo_url":"https://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:     runningRun,
			mockRepoErr: pgx.ErrTxClosed,
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:        tc.mockRun,
				getRunErr:           tc.mockRunErr,
				createRunRepoResult: tc.mockRepoRes,
				createRunRepoErr:    tc.mockRepoErr,
			}

			handler := addRunRepoHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/repos", strings.NewReader(tc.body))
			req.SetPathValue("id", tc.runID)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var resp RunRepoResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if resp.ID != tc.wantRepoID {
					t.Errorf("repo id = %s, want %s", resp.ID, tc.wantRepoID)
				}
				if resp.Status != "pending" {
					t.Errorf("status = %s, want pending", resp.Status)
				}
			}

			if tc.wantCallStore && !m.createRunRepoCalled {
				t.Error("expected CreateRunRepo to be called")
			}
		})
	}
}

// TestListRunReposHandler verifies the GET /v1/runs/{id}/repos handler.
func TestListRunReposHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	sampleRepoID1 := uuid.New()
	sampleRepoID2 := uuid.New()

	sampleRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	sampleRepos := []store.RunRepo{
		{
			ID:        pgtype.UUID{Bytes: sampleRepoID1, Valid: true},
			RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
			RepoUrl:   "https://github.com/example/repo1.git",
			BaseRef:   "main",
			TargetRef: "feature-1",
			Status:    store.RunRepoStatusPending,
			Attempt:   1,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		{
			ID:        pgtype.UUID{Bytes: sampleRepoID2, Valid: true},
			RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
			RepoUrl:   "https://github.com/example/repo2.git",
			BaseRef:   "main",
			TargetRef: "feature-2",
			Status:    store.RunRepoStatusSucceeded,
			Attempt:   1,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	tests := []struct {
		name          string
		runID         string
		mockRun       store.Run
		mockRunErr    error
		mockRepos     []store.RunRepo
		mockReposErr  error
		wantStatus    int
		wantRepoCount int
	}{
		{
			name:          "list repos successfully",
			runID:         sampleRunID.String(),
			mockRun:       sampleRun,
			mockRepos:     sampleRepos,
			wantStatus:    http.StatusOK,
			wantRepoCount: 2,
		},
		{
			name:          "empty list",
			runID:         sampleRunID.String(),
			mockRun:       sampleRun,
			mockRepos:     []store.RunRepo{},
			wantStatus:    http.StatusOK,
			wantRepoCount: 0,
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
			name:         "list repos error",
			runID:        sampleRunID.String(),
			mockRun:      sampleRun,
			mockReposErr: pgx.ErrTxClosed,
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:            tc.mockRun,
				getRunErr:               tc.mockRunErr,
				listRunReposByRunResult: tc.mockRepos,
				listRunReposByRunErr:    tc.mockReposErr,
			}

			handler := listRunReposHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tc.runID+"/repos", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var resp struct {
					Repos []RunRepoResponse `json:"repos"`
				}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(resp.Repos) != tc.wantRepoCount {
					t.Errorf("repo count = %d, want %d", len(resp.Repos), tc.wantRepoCount)
				}
			}
		})
	}
}

// TestDeleteRunRepoHandler verifies the DELETE /v1/runs/{id}/repos/{repo_id} handler.
func TestDeleteRunRepoHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	sampleRepoID := uuid.New()

	sampleRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	runningRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusRunning,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	succeededRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusSucceeded,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Repo that belongs to a different run.
	differentRunID := uuid.New()
	differentRunRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: differentRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runID            string
		repoID           string
		mockRun          store.Run
		mockRunErr       error
		mockRepo         store.RunRepo
		mockRepoErr      error
		updateStatusErr  error
		wantStatus       int
		wantNewStatus    string
		wantStatusUpdate bool
	}{
		{
			name:             "delete pending repo (skipped)",
			runID:            sampleRunID.String(),
			repoID:           sampleRepoID.String(),
			mockRun:          sampleRun,
			mockRepo:         pendingRepo,
			wantStatus:       http.StatusOK,
			wantNewStatus:    "skipped",
			wantStatusUpdate: true,
		},
		{
			name:             "delete running repo (cancelled)",
			runID:            sampleRunID.String(),
			repoID:           sampleRepoID.String(),
			mockRun:          sampleRun,
			mockRepo:         runningRepo,
			wantStatus:       http.StatusOK,
			wantNewStatus:    "cancelled",
			wantStatusUpdate: true,
		},
		{
			name:             "delete already terminal repo (idempotent)",
			runID:            sampleRunID.String(),
			repoID:           sampleRepoID.String(),
			mockRun:          sampleRun,
			mockRepo:         succeededRepo,
			wantStatus:       http.StatusOK,
			wantNewStatus:    "succeeded",
			wantStatusUpdate: false,
		},
		{
			name:       "empty run id",
			runID:      "",
			repoID:     sampleRepoID.String(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty repo id",
			runID:      sampleRunID.String(),
			repoID:     "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid run uuid",
			runID:      "not-a-uuid",
			repoID:     sampleRepoID.String(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid repo uuid",
			runID:      sampleRunID.String(),
			repoID:     "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "run not found",
			runID:      uuid.New().String(),
			repoID:     sampleRepoID.String(),
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "repo not found",
			runID:       sampleRunID.String(),
			repoID:      uuid.New().String(),
			mockRun:     sampleRun,
			mockRepoErr: pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:       "repo belongs to different run",
			runID:      sampleRunID.String(),
			repoID:     sampleRepoID.String(),
			mockRun:    sampleRun,
			mockRepo:   differentRunRepo,
			wantStatus: http.StatusNotFound,
		},
		{
			name:             "update status error",
			runID:            sampleRunID.String(),
			repoID:           sampleRepoID.String(),
			mockRun:          sampleRun,
			mockRepo:         pendingRepo,
			updateStatusErr:  pgx.ErrTxClosed,
			wantStatus:       http.StatusInternalServerError,
			wantStatusUpdate: true, // We do call it, but it fails.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:           tc.mockRun,
				getRunErr:              tc.mockRunErr,
				getRunRepoResult:       tc.mockRepo,
				getRunRepoErr:          tc.mockRepoErr,
				updateRunRepoStatusErr: tc.updateStatusErr,
			}

			handler := deleteRunRepoHandler(m)
			req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+tc.runID+"/repos/"+tc.repoID, nil)
			req.SetPathValue("id", tc.runID)
			req.SetPathValue("repo_id", tc.repoID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// For successful updates, verify UpdateRunRepoStatus was called with correct params.
			if tc.wantStatusUpdate {
				if !m.updateRunRepoStatusCalled {
					t.Error("expected UpdateRunRepoStatus to be called")
				} else if tc.wantNewStatus != "" && len(m.updateRunRepoStatusParams) > 0 {
					// Only verify the status if we expect a specific one.
					updatedStatus := string(m.updateRunRepoStatusParams[0].Status)
					if updatedStatus != tc.wantNewStatus {
						t.Errorf("updated status = %s, want %s", updatedStatus, tc.wantNewStatus)
					}
				}
			}
			if !tc.wantStatusUpdate && m.updateRunRepoStatusCalled {
				t.Error("expected UpdateRunRepoStatus NOT to be called")
			}
		})
	}
}

// TestRestartRunRepoHandler verifies the POST /v1/runs/{id}/repos/{repo_id}/restart handler.
func TestRestartRunRepoHandler(t *testing.T) {
	t.Parallel()

	sampleRunID := uuid.New()
	sampleRepoID := uuid.New()

	runningRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleRunID, Valid: true},
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	failedRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusFailed,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	restartedRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   2,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Repo that belongs to a different run.
	differentRunID := uuid.New()
	differentRunRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID, Valid: true},
		RunID:     pgtype.UUID{Bytes: differentRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusFailed,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name                 string
		runID                string
		repoID               string
		body                 string
		mockRun              store.Run
		mockRunErr           error
		mockRepo             store.RunRepo
		mockRepoErr          error
		mockRestartedRepo    store.RunRepo
		incrementAttemptErr  error
		updateRefsErr        error
		wantStatus           int
		wantIncrementAttempt bool
		wantAttempt          int32
		wantUpdateRefs       bool
		wantBaseRef          string
		wantTargetRef        string
	}{
		{
			name:                 "restart failed repo",
			runID:                sampleRunID.String(),
			repoID:               sampleRepoID.String(),
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       false,
		},
		{
			name:                 "restart with empty body",
			runID:                sampleRunID.String(),
			repoID:               sampleRepoID.String(),
			body:                 "",
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       false,
		},
		{
			name:                 "restart with new refs",
			runID:                sampleRunID.String(),
			repoID:               sampleRepoID.String(),
			body:                 `{"base_ref":"main-updated","target_ref":"feature-updated"}`,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       true,
			wantBaseRef:          "main-updated",
			wantTargetRef:        "feature-updated",
		},
		{
			name:       "empty run id",
			runID:      "",
			repoID:     sampleRepoID.String(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty repo id",
			runID:      sampleRunID.String(),
			repoID:     "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid run uuid",
			runID:      "not-a-uuid",
			repoID:     sampleRepoID.String(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid repo uuid",
			runID:      sampleRunID.String(),
			repoID:     "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "run not found",
			runID:      uuid.New().String(),
			repoID:     sampleRepoID.String(),
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "run is terminal",
			runID:      sampleRunID.String(),
			repoID:     sampleRepoID.String(),
			mockRun:    canceledRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:        "repo not found",
			runID:       sampleRunID.String(),
			repoID:      uuid.New().String(),
			mockRun:     runningRun,
			mockRepoErr: pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:       "repo belongs to different run",
			runID:      sampleRunID.String(),
			repoID:     sampleRepoID.String(),
			mockRun:    runningRun,
			mockRepo:   differentRunRepo,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "cannot restart pending repo",
			runID:      sampleRunID.String(),
			repoID:     sampleRepoID.String(),
			mockRun:    runningRun,
			mockRepo:   pendingRepo,
			wantStatus: http.StatusConflict,
		},
		{
			name:                 "increment attempt error",
			runID:                sampleRunID.String(),
			repoID:               sampleRepoID.String(),
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			incrementAttemptErr:  pgx.ErrTxClosed,
			wantStatus:           http.StatusInternalServerError,
			wantIncrementAttempt: true, // We do call it, but it fails.
			wantUpdateRefs:       false,
		},
		{
			name:                 "update refs error",
			runID:                sampleRunID.String(),
			repoID:               sampleRepoID.String(),
			body:                 `{"base_ref":"main-updated","target_ref":"feature-updated"}`,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			updateRefsErr:        pgx.ErrTxClosed,
			wantStatus:           http.StatusInternalServerError,
			wantIncrementAttempt: true,
			wantUpdateRefs:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mock returns the restarted repo on second GetRunRepo call (after increment).
			mockRepoOnGet := tc.mockRepo
			if tc.mockRestartedRepo.ID.Valid {
				mockRepoOnGet = tc.mockRestartedRepo
			}

			m := &mockStore{
				getRunResult:               tc.mockRun,
				getRunErr:                  tc.mockRunErr,
				getRunRepoResult:           mockRepoOnGet,
				getRunRepoErr:              tc.mockRepoErr,
				incrementRunRepoAttemptErr: tc.incrementAttemptErr,
				updateRunRepoRefsErr:       tc.updateRefsErr,
			}

			// On successful restart, GetRunRepo is called twice (before and after increment).
			// First call returns the original repo, second returns restarted.
			// For simplicity, we set the result to the restarted repo if available.
			if tc.mockRepo.ID.Valid && tc.mockRestartedRepo.ID.Valid {
				m.getRunRepoResult = tc.mockRepo
			}

			handler := restartRunRepoHandler(m)
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/repos/"+tc.repoID+"/restart", body)
			req.SetPathValue("id", tc.runID)
			req.SetPathValue("repo_id", tc.repoID)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantIncrementAttempt && !m.incrementRunRepoAttemptCalled {
				t.Error("expected IncrementRunRepoAttempt to be called")
			}
			if !tc.wantIncrementAttempt && m.incrementRunRepoAttemptCalled {
				t.Error("expected IncrementRunRepoAttempt NOT to be called")
			}

			if tc.wantUpdateRefs {
				if !m.updateRunRepoRefsCalled {
					t.Error("expected UpdateRunRepoRefs to be called")
				} else {
					if tc.wantBaseRef != "" && m.updateRunRepoRefsParams.BaseRef != tc.wantBaseRef {
						t.Errorf("base_ref = %s, want %s", m.updateRunRepoRefsParams.BaseRef, tc.wantBaseRef)
					}
					if tc.wantTargetRef != "" && m.updateRunRepoRefsParams.TargetRef != tc.wantTargetRef {
						t.Errorf("target_ref = %s, want %s", m.updateRunRepoRefsParams.TargetRef, tc.wantTargetRef)
					}
				}
			} else if m.updateRunRepoRefsCalled {
				t.Error("expected UpdateRunRepoRefs NOT to be called")
			}
		})
	}
}

// TestIsTerminalRunRepoStatus verifies the helper function.
func TestIsTerminalRunRepoStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status store.RunRepoStatus
		want   bool
	}{
		{store.RunRepoStatusPending, false},
		{store.RunRepoStatusRunning, false},
		{store.RunRepoStatusSucceeded, true},
		{store.RunRepoStatusFailed, true},
		{store.RunRepoStatusSkipped, true},
		{store.RunRepoStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := isTerminalRunRepoStatus(tc.status); got != tc.want {
				t.Errorf("isTerminalRunRepoStatus(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}
