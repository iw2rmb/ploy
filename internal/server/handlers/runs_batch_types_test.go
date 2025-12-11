package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

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

	runID := domaintypes.NewRunID().String()

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

			counts, err := getRunRepoCounts(context.Background(), m, domaintypes.RunID(runID))
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

// TestRunSummary_JSONSerialization verifies that RunSummary correctly
// serializes fields to JSON strings and preserves type information during
// round-trip encoding/decoding.
func TestRunSummary_JSONSerialization(t *testing.T) {
	t.Parallel()

	summary := RunSummary{
		ID:        domaintypes.RunID("12345678-1234-1234-1234-123456789abc"),
		Name:      ptrString("test-batch"),
		Status:    "running",
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
		t.Fatalf("marshal RunSummary: %v", err)
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
	var decoded RunSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal RunSummary: %v", err)
	}

	// Verify fields match after round-trip.
	if decoded.ID.String() != summary.ID.String() {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID.String(), summary.ID.String())
	}
	if decoded.RepoURL != summary.RepoURL {
		t.Errorf("RepoURL mismatch: got %s, want %s", decoded.RepoURL, summary.RepoURL)
	}
	if decoded.BaseRef != summary.BaseRef {
		t.Errorf("BaseRef mismatch: got %s, want %s", decoded.BaseRef, summary.BaseRef)
	}
	if decoded.TargetRef != summary.TargetRef {
		t.Errorf("TargetRef mismatch: got %s, want %s", decoded.TargetRef, summary.TargetRef)
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
