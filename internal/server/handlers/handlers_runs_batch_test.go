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
				// Compare domain type ID with expected string using .String() method.
				if resp.ID.String() != tc.runID {
					t.Errorf("id = %s, want %s", resp.ID.String(), tc.runID)
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

// TestGetRunRepoCounts verifies the getRunRepoCounts helper.
// Tests both count aggregation and derived status computation.
func TestGetRunRepoCounts(t *testing.T) {
	t.Parallel()

	runID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	tests := []struct {
		name              string
		mockCounts        []store.CountRunReposByStatusRow
		mockErr           error
		wantTotal         int32
		wantDerivedStatus string
		wantErr           bool
	}{
		{
			name: "all statuses — cancelled takes precedence",
			mockCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 1},
				{Status: store.RunRepoStatusRunning, Count: 2},
				{Status: store.RunRepoStatusSucceeded, Count: 3},
				{Status: store.RunRepoStatusFailed, Count: 4},
				{Status: store.RunRepoStatusSkipped, Count: 5},
				{Status: store.RunRepoStatusCancelled, Count: 6},
			},
			wantTotal:         21,
			wantDerivedStatus: DerivedStatusCancelled,
		},
		{
			name: "running repos — derived running",
			mockCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 2},
				{Status: store.RunRepoStatusRunning, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusRunning,
		},
		{
			name: "all succeeded — derived completed",
			mockCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSucceeded, Count: 3},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusCompleted,
		},
		{
			name: "some failed — derived failed",
			mockCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSucceeded, Count: 2},
				{Status: store.RunRepoStatusFailed, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusFailed,
		},
		{
			name:              "empty — derived pending",
			mockCounts:        []store.CountRunReposByStatusRow{},
			wantTotal:         0,
			wantDerivedStatus: DerivedStatusPending,
		},
		{
			name:    "error propagates",
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

			if counts.DerivedStatus != tc.wantDerivedStatus {
				t.Errorf("derived_status = %q, want %q", counts.DerivedStatus, tc.wantDerivedStatus)
			}
		})
	}
}

// TestDeriveBatchStatus verifies the deriveBatchStatus helper function.
// This function computes a single batch-level status from repo counts.
func TestDeriveBatchStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		counts *RunRepoCounts
		want   string
	}{
		// Empty batch — pending (no work yet).
		{
			name:   "empty batch",
			counts: &RunRepoCounts{Total: 0},
			want:   DerivedStatusPending,
		},
		// All pending — pending.
		{
			name: "all pending",
			counts: &RunRepoCounts{
				Total:   3,
				Pending: 3,
			},
			want: DerivedStatusPending,
		},
		// Some running — running (active execution).
		{
			name: "one running rest pending",
			counts: &RunRepoCounts{
				Total:   3,
				Pending: 2,
				Running: 1,
			},
			want: DerivedStatusRunning,
		},
		{
			name: "all running",
			counts: &RunRepoCounts{
				Total:   2,
				Running: 2,
			},
			want: DerivedStatusRunning,
		},
		{
			name: "running with some succeeded",
			counts: &RunRepoCounts{
				Total:     3,
				Running:   1,
				Succeeded: 2,
			},
			want: DerivedStatusRunning,
		},
		// All succeeded — completed.
		{
			name: "all succeeded",
			counts: &RunRepoCounts{
				Total:     3,
				Succeeded: 3,
			},
			want: DerivedStatusCompleted,
		},
		// Succeeded + skipped — completed.
		{
			name: "succeeded and skipped",
			counts: &RunRepoCounts{
				Total:     4,
				Succeeded: 2,
				Skipped:   2,
			},
			want: DerivedStatusCompleted,
		},
		// All skipped — completed.
		{
			name: "all skipped",
			counts: &RunRepoCounts{
				Total:   2,
				Skipped: 2,
			},
			want: DerivedStatusCompleted,
		},
		// Some failed — failed.
		{
			name: "one failed rest succeeded",
			counts: &RunRepoCounts{
				Total:     3,
				Succeeded: 2,
				Failed:    1,
			},
			want: DerivedStatusFailed,
		},
		{
			name: "all failed",
			counts: &RunRepoCounts{
				Total:  2,
				Failed: 2,
			},
			want: DerivedStatusFailed,
		},
		{
			name: "failed with skipped",
			counts: &RunRepoCounts{
				Total:   3,
				Failed:  1,
				Skipped: 2,
			},
			want: DerivedStatusFailed,
		},
		// Cancelled takes precedence over other terminal states.
		{
			name: "cancelled alone",
			counts: &RunRepoCounts{
				Total:     2,
				Cancelled: 2,
			},
			want: DerivedStatusCancelled,
		},
		{
			name: "cancelled with succeeded",
			counts: &RunRepoCounts{
				Total:     3,
				Cancelled: 1,
				Succeeded: 2,
			},
			want: DerivedStatusCancelled,
		},
		{
			name: "cancelled with failed",
			counts: &RunRepoCounts{
				Total:     3,
				Cancelled: 1,
				Failed:    2,
			},
			want: DerivedStatusCancelled,
		},
		{
			name: "cancelled with pending",
			counts: &RunRepoCounts{
				Total:     3,
				Cancelled: 1,
				Pending:   2,
			},
			want: DerivedStatusCancelled,
		},
		// Running takes precedence over failed (still active work).
		{
			name: "running with failed",
			counts: &RunRepoCounts{
				Total:   3,
				Running: 1,
				Failed:  2,
			},
			want: DerivedStatusRunning,
		},
		// Mixed: pending with succeeded — still pending (not all started).
		{
			name: "pending with succeeded",
			counts: &RunRepoCounts{
				Total:     3,
				Pending:   1,
				Succeeded: 2,
			},
			want: DerivedStatusPending,
		},
		// All six statuses present — cancelled takes precedence.
		{
			name: "all statuses",
			counts: &RunRepoCounts{
				Total:     21,
				Pending:   1,
				Running:   2,
				Succeeded: 3,
				Failed:    4,
				Skipped:   5,
				Cancelled: 6,
			},
			want: DerivedStatusCancelled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := deriveBatchStatus(tc.counts)
			if got != tc.want {
				t.Errorf("deriveBatchStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ptrString returns a pointer to the given string.
func ptrString(s string) *string {
	return &s
}

// TestRunBatchSummary_JSONSerialization verifies that RunBatchSummary correctly
// serializes domain types to JSON strings and preserves type information during
// round-trip encoding/decoding.
func TestRunBatchSummary_JSONSerialization(t *testing.T) {
	t.Parallel()

	summary := RunBatchSummary{
		ID:        "12345678-1234-1234-1234-123456789abc",
		Name:      ptrString("test-batch"),
		Status:    store.RunStatusRunning,
		RepoURL:   "https://github.com/example/repo.git",
		BaseRef:   "main",
		TargetRef: "feature-branch",
		CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Counts: &RunRepoCounts{
			Total:         5,
			Pending:       2,
			Running:       1,
			Succeeded:     2,
			DerivedStatus: DerivedStatusRunning,
		},
	}

	// Marshal to JSON.
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal RunBatchSummary: %v", err)
	}

	// Verify JSON contains expected string values (domain types serialize as strings).
	jsonStr := string(data)
	checks := []struct {
		field string
		want  string
	}{
		{"id", `"id":"12345678-1234-1234-1234-123456789abc"`},
		{"repo_url", `"repo_url":"https://github.com/example/repo.git"`},
		{"base_ref", `"base_ref":"main"`},
		{"target_ref", `"target_ref":"feature-branch"`},
		{"status", `"status":"running"`},
	}
	for _, tc := range checks {
		if !strings.Contains(jsonStr, tc.want) {
			t.Errorf("JSON missing %s: got %s", tc.field, jsonStr)
		}
	}

	// Round-trip: unmarshal back to verify domain types decode correctly.
	var decoded RunBatchSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal RunBatchSummary: %v", err)
	}

	// Verify fields match after round-trip.
	if decoded.ID.String() != summary.ID.String() {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID.String(), summary.ID.String())
	}
	if decoded.RepoURL.String() != summary.RepoURL.String() {
		t.Errorf("RepoURL mismatch: got %s, want %s", decoded.RepoURL.String(), summary.RepoURL.String())
	}
	if decoded.BaseRef.String() != summary.BaseRef.String() {
		t.Errorf("BaseRef mismatch: got %s, want %s", decoded.BaseRef.String(), summary.BaseRef.String())
	}
	if decoded.TargetRef.String() != summary.TargetRef.String() {
		t.Errorf("TargetRef mismatch: got %s, want %s", decoded.TargetRef.String(), summary.TargetRef.String())
	}
}

// TestRunRepoResponse_JSONSerialization verifies that RunRepoResponse correctly
// serializes domain types (RunRepoID, RunID, RepoURL, GitRef) to JSON strings.
func TestRunRepoResponse_JSONSerialization(t *testing.T) {
	t.Parallel()

	resp := RunRepoResponse{
		ID:        "repo-12345678-1234-1234-1234-123456789abc",
		RunID:     "run-12345678-1234-1234-1234-123456789abc",
		RepoURL:   "https://github.com/example/repo.git",
		BaseRef:   "main",
		TargetRef: "feature",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Marshal to JSON.
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal RunRepoResponse: %v", err)
	}

	// Verify JSON contains expected string values.
	jsonStr := string(data)
	checks := []struct {
		field string
		want  string
	}{
		{"id", `"id":"repo-12345678-1234-1234-1234-123456789abc"`},
		{"run_id", `"run_id":"run-12345678-1234-1234-1234-123456789abc"`},
		{"repo_url", `"repo_url":"https://github.com/example/repo.git"`},
		{"base_ref", `"base_ref":"main"`},
		{"target_ref", `"target_ref":"feature"`},
		{"status", `"status":"pending"`},
	}
	for _, tc := range checks {
		if !strings.Contains(jsonStr, tc.want) {
			t.Errorf("JSON missing %s: got %s", tc.field, jsonStr)
		}
	}

	// Round-trip: unmarshal back to verify domain types decode correctly.
	var decoded RunRepoResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal RunRepoResponse: %v", err)
	}

	// Verify typed fields match after round-trip.
	if decoded.ID.String() != resp.ID.String() {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID.String(), resp.ID.String())
	}
	if decoded.RunID.String() != resp.RunID.String() {
		t.Errorf("RunID mismatch: got %s, want %s", decoded.RunID.String(), resp.RunID.String())
	}
	if decoded.RepoURL.String() != resp.RepoURL.String() {
		t.Errorf("RepoURL mismatch: got %s, want %s", decoded.RepoURL.String(), resp.RepoURL.String())
	}
	if decoded.Status != resp.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, resp.Status)
	}
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
				// Compare domain type ID with expected string using .String() method.
				if resp.ID.String() != tc.wantRepoID {
					t.Errorf("repo id = %s, want %s", resp.ID.String(), tc.wantRepoID)
				}
				// Compare typed RunRepoStatus with expected value.
				if resp.Status != store.RunRepoStatusPending {
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

// -------------------------------------------------------------------------
// Tests for POST /v1/runs/{id}/start — batch execution start handler.
// -------------------------------------------------------------------------

// TestStartRunHandler verifies the POST /v1/runs/{id}/start handler.
func TestStartRunHandler(t *testing.T) {
	t.Parallel()

	sampleBatchRunID := uuid.New()
	sampleRepoID1 := uuid.New()
	sampleRepoID2 := uuid.New()
	childRunID := uuid.New()

	// Sample batch run (queued, ready to start).
	queuedBatchRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/batch.git",
		Spec:      []byte(`{"mod":{"image":"test-image"}}`),
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedBy: ptrString("test-user"),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample canceled batch run (terminal, cannot start).
	canceledBatchRun := store.Run{
		ID:        pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		Name:      ptrString("test-batch"),
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample pending run repos.
	pendingRepo1 := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID1, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo1.git",
		BaseRef:   "main",
		TargetRef: "feature-1",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo2 := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID2, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo2.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample already succeeded repo.
	succeededRepo := store.RunRepo{
		ID:        pgtype.UUID{Bytes: sampleRepoID1, Valid: true},
		RunID:     pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo1.git",
		Status:    store.RunRepoStatusSucceeded,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample child run created for execution.
	childRun := store.Run{
		ID:        pgtype.UUID{Bytes: childRunID, Valid: true},
		RepoUrl:   "https://github.com/example/repo1.git",
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature-1",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name                  string
		runID                 string
		mockRun               store.Run
		mockRunErr            error
		mockAllRepos          []store.RunRepo
		mockAllReposErr       error
		mockPendingRepos      []store.RunRepo
		mockPendingReposErr   error
		mockChildRun          store.Run
		mockCreateRunErr      error
		mockCreateJobErr      error
		mockSetExecRunErr     error
		wantStatus            int
		wantStarted           int
		wantAlreadyDone       int
		wantPending           int
		wantChildRunsCreated  int
		wantSetExecRunCalled  bool
		wantAckRunStartCalled bool
	}{
		{
			name:                  "start two pending repos",
			runID:                 sampleBatchRunID.String(),
			mockRun:               queuedBatchRun,
			mockAllRepos:          []store.RunRepo{pendingRepo1, pendingRepo2},
			mockPendingRepos:      []store.RunRepo{pendingRepo1, pendingRepo2},
			mockChildRun:          childRun,
			wantStatus:            http.StatusOK,
			wantStarted:           2,
			wantAlreadyDone:       0,
			wantPending:           0,
			wantChildRunsCreated:  2,
			wantSetExecRunCalled:  true,
			wantAckRunStartCalled: true,
		},
		{
			name:                 "no pending repos (all succeeded)",
			runID:                sampleBatchRunID.String(),
			mockRun:              queuedBatchRun,
			mockAllRepos:         []store.RunRepo{succeededRepo},
			mockPendingRepos:     []store.RunRepo{},
			wantStatus:           http.StatusOK,
			wantStarted:          0,
			wantAlreadyDone:      1,
			wantPending:          0,
			wantChildRunsCreated: 0,
			wantSetExecRunCalled: false,
		},
		{
			name:       "empty run id",
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
			name:       "terminal run (conflict)",
			runID:      sampleBatchRunID.String(),
			mockRun:    canceledBatchRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:            "list all repos error",
			runID:           sampleBatchRunID.String(),
			mockRun:         queuedBatchRun,
			mockAllReposErr: pgx.ErrTxClosed,
			wantStatus:      http.StatusInternalServerError,
		},
		{
			name:                "list pending repos error",
			runID:               sampleBatchRunID.String(),
			mockRun:             queuedBatchRun,
			mockAllRepos:        []store.RunRepo{pendingRepo1},
			mockPendingReposErr: pgx.ErrTxClosed,
			wantStatus:          http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:                   tc.mockRun,
				getRunErr:                      tc.mockRunErr,
				listRunReposByRunResult:        tc.mockAllRepos,
				listRunReposByRunErr:           tc.mockAllReposErr,
				listPendingRunReposByRunResult: tc.mockPendingRepos,
				listPendingRunReposByRunErr:    tc.mockPendingReposErr,
				createRunResult:                tc.mockChildRun,
				createRunErr:                   tc.mockCreateRunErr,
				createJobErr:                   tc.mockCreateJobErr,
				setRunRepoExecutionRunErr:      tc.mockSetExecRunErr,
			}

			handler := startRunHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/start", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// For successful responses, verify response body.
			if tc.wantStatus == http.StatusOK {
				var resp StartRunResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if resp.Started != tc.wantStarted {
					t.Errorf("started = %d, want %d", resp.Started, tc.wantStarted)
				}
				if resp.AlreadyDone != tc.wantAlreadyDone {
					t.Errorf("already_done = %d, want %d", resp.AlreadyDone, tc.wantAlreadyDone)
				}
				if resp.Pending != tc.wantPending {
					t.Errorf("pending = %d, want %d", resp.Pending, tc.wantPending)
				}
			}

			// Verify child runs were created.
			if tc.wantChildRunsCreated > 0 {
				if !m.createRunCalled {
					t.Error("expected CreateRun to be called")
				}
			}

			// Verify SetRunRepoExecutionRun was called.
			if tc.wantSetExecRunCalled && !m.setRunRepoExecutionRunCalled {
				t.Error("expected SetRunRepoExecutionRun to be called")
			}

			// Verify AckRunStart was called for batch transition.
			if tc.wantAckRunStartCalled && !m.ackRunStartCalled {
				t.Error("expected AckRunStart to be called")
			}
		})
	}
}

// TestMaybeUpdateRunRepoFromExecution verifies the completion callback updates RunRepo status.
func TestMaybeUpdateRunRepoFromExecution(t *testing.T) {
	t.Parallel()

	sampleRunRepoID := uuid.New()
	sampleBatchRunID := uuid.New()
	sampleExecutionRunID := uuid.New()

	linkedRunRepo := store.RunRepo{
		ID:             pgtype.UUID{Bytes: sampleRunRepoID, Valid: true},
		RunID:          pgtype.UUID{Bytes: sampleBatchRunID, Valid: true},
		ExecutionRunID: pgtype.UUID{Bytes: sampleExecutionRunID, Valid: true},
		RepoUrl:        "https://github.com/example/repo.git",
		Status:         store.RunRepoStatusRunning,
		Attempt:        1,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runStatus        store.RunStatus
		mockRunRepo      store.RunRepo
		mockRunRepoErr   error
		mockUpdateErr    error
		wantRepoStatus   store.RunRepoStatus
		wantUpdateCalled bool
		wantErr          bool
	}{
		{
			name:             "succeeded execution updates repo to succeeded",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusSucceeded,
			wantUpdateCalled: true,
		},
		{
			name:             "failed execution updates repo to failed",
			runStatus:        store.RunStatusFailed,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusFailed,
			wantUpdateCalled: true,
		},
		{
			name:             "canceled execution updates repo to cancelled",
			runStatus:        store.RunStatusCanceled,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusCancelled,
			wantUpdateCalled: true,
		},
		{
			name:             "standalone run (no linked run_repo) — no update",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepoErr:   pgx.ErrNoRows,
			wantUpdateCalled: false,
			wantErr:          false, // Not an error — expected for standalone runs.
		},
		{
			name:             "lookup error propagates",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepoErr:   pgx.ErrTxClosed,
			wantUpdateCalled: false,
			wantErr:          true,
		},
		{
			name:             "update error propagates",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepo:      linkedRunRepo,
			mockUpdateErr:    pgx.ErrTxClosed,
			wantRepoStatus:   store.RunRepoStatusSucceeded, // We still try to update to succeeded.
			wantUpdateCalled: true,
			wantErr:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunRepoByExecutionRunResult: tc.mockRunRepo,
				getRunRepoByExecutionRunErr:    tc.mockRunRepoErr,
				updateRunRepoStatusErr:         tc.mockUpdateErr,
			}

			execRunID := pgtype.UUID{Bytes: sampleExecutionRunID, Valid: true}
			err := maybeUpdateRunRepoFromExecution(context.Background(), m, execRunID, tc.runStatus)

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tc.wantUpdateCalled {
				if !m.updateRunRepoStatusCalled {
					t.Error("expected UpdateRunRepoStatus to be called")
				} else if len(m.updateRunRepoStatusParams) > 0 {
					updatedStatus := m.updateRunRepoStatusParams[0].Status
					if updatedStatus != tc.wantRepoStatus {
						t.Errorf("updated status = %s, want %s", updatedStatus, tc.wantRepoStatus)
					}
				}
			} else if m.updateRunRepoStatusCalled {
				t.Error("expected UpdateRunRepoStatus NOT to be called")
			}
		})
	}
}
