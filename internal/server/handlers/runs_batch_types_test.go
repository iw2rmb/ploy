package handlers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestIsTerminalRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status store.RunStatus
		want   bool
	}{
		{store.RunStatusStarted, false},
		{store.RunStatusFinished, true},
		{store.RunStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := isTerminalRunStatus(tc.status); got != tc.want {
				t.Fatalf("isTerminalRunStatus(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestIsTerminalRunRepoStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status store.RunRepoStatus
		want   bool
	}{
		{store.RunRepoStatusQueued, false},
		{store.RunRepoStatusRunning, false},
		{store.RunRepoStatusSuccess, true},
		{store.RunRepoStatusFail, true},
		{store.RunRepoStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := isTerminalRunRepoStatus(tc.status); got != tc.want {
				t.Fatalf("isTerminalRunRepoStatus(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestGetRunRepoCounts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	tests := []struct {
		name              string
		rows              []store.CountRunReposByStatusRow
		err               error
		wantTotal         int32
		wantDerivedStatus string
		wantErr           bool
	}{
		{
			name: "cancelled takes precedence",
			rows: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusQueued, Count: 1},
				{Status: store.RunRepoStatusRunning, Count: 1},
				{Status: store.RunRepoStatusCancelled, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusCancelled,
		},
		{
			name: "running when any running",
			rows: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusQueued, Count: 2},
				{Status: store.RunRepoStatusRunning, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusRunning,
		},
		{
			name: "failed when any fail and none running/cancelled",
			rows: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSuccess, Count: 2},
				{Status: store.RunRepoStatusFail, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusFailed,
		},
		{
			name: "completed when all terminal and no fail/cancelled",
			rows: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSuccess, Count: 3},
			},
			wantTotal:         3,
			wantDerivedStatus: DerivedStatusCompleted,
		},
		{
			name:              "pending when queued only",
			rows:              []store.CountRunReposByStatusRow{{Status: store.RunRepoStatusQueued, Count: 2}},
			wantTotal:         2,
			wantDerivedStatus: DerivedStatusPending,
		},
		{
			name:    "error propagates",
			err:     pgx.ErrTxClosed,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st := &mockStore{
				countRunReposByStatusResult: tc.rows,
				countRunReposByStatusErr:    tc.err,
			}

			counts, err := getRunRepoCounts(context.Background(), st, runID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if counts.Total != tc.wantTotal {
				t.Fatalf("total=%d, want %d", counts.Total, tc.wantTotal)
			}
			if counts.DerivedStatus != tc.wantDerivedStatus {
				t.Fatalf("derived_status=%q, want %q", counts.DerivedStatus, tc.wantDerivedStatus)
			}
		})
	}
}

func TestDeriveBatchStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		counts *domaintypes.RunRepoCounts
		want   string
	}{
		{name: "empty batch", counts: &domaintypes.RunRepoCounts{Total: 0}, want: DerivedStatusPending},
		{name: "queued only", counts: &domaintypes.RunRepoCounts{Total: 2, Queued: 2}, want: DerivedStatusPending},
		{name: "running", counts: &domaintypes.RunRepoCounts{Total: 2, Running: 1, Queued: 1}, want: DerivedStatusRunning},
		{name: "cancelled", counts: &domaintypes.RunRepoCounts{Total: 2, Cancelled: 1, Running: 1}, want: DerivedStatusCancelled},
		{name: "failed", counts: &domaintypes.RunRepoCounts{Total: 2, Fail: 1, Success: 1}, want: DerivedStatusFailed},
		{name: "completed", counts: &domaintypes.RunRepoCounts{Total: 2, Success: 2}, want: DerivedStatusCompleted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveBatchStatus(tc.counts); got != tc.want {
				t.Fatalf("deriveBatchStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}
