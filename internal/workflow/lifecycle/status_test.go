package lifecycle_test

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func TestIsTerminalRunStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status domaintypes.RunStatus
		want   bool
	}{
		{domaintypes.RunStatusStarted, false},
		{domaintypes.RunStatusFinished, true},
		{domaintypes.RunStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.IsTerminalRunStatus(tc.status); got != tc.want {
				t.Fatalf("IsTerminalRunStatus(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestIsTerminalRunRepoStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status domaintypes.RunRepoStatus
		want   bool
	}{
		{domaintypes.RunRepoStatusQueued, false},
		{domaintypes.RunRepoStatusRunning, false},
		{domaintypes.RunRepoStatusSuccess, true},
		{domaintypes.RunRepoStatusFail, true},
		{domaintypes.RunRepoStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.IsTerminalRunRepoStatus(tc.status); got != tc.want {
				t.Fatalf("IsTerminalRunRepoStatus(%s) = %v, want %v", tc.status, got, tc.want)
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
		{name: "empty batch", counts: &domaintypes.RunRepoCounts{Total: 0}, want: lifecycle.DerivedStatusPending},
		{name: "queued only", counts: &domaintypes.RunRepoCounts{Total: 2, Queued: 2}, want: lifecycle.DerivedStatusPending},
		{name: "running", counts: &domaintypes.RunRepoCounts{Total: 2, Running: 1, Queued: 1}, want: lifecycle.DerivedStatusRunning},
		{name: "cancelled", counts: &domaintypes.RunRepoCounts{Total: 2, Cancelled: 1, Running: 1}, want: lifecycle.DerivedStatusCancelled},
		{name: "failed", counts: &domaintypes.RunRepoCounts{Total: 2, Fail: 1, Success: 1}, want: lifecycle.DerivedStatusFailed},
		{name: "completed", counts: &domaintypes.RunRepoCounts{Total: 2, Success: 2}, want: lifecycle.DerivedStatusCompleted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := lifecycle.DeriveBatchStatus(tc.counts); got != tc.want {
				t.Fatalf("DeriveBatchStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEvaluateRunCompletionFromRepoCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		counts     []store.CountRunReposByStatusRow
		wantFinish bool
		wantState  migsapi.RunState
	}{
		{
			name: "all success",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 2},
			},
			wantFinish: true,
			wantState:  migsapi.RunStateSucceeded,
		},
		{
			name: "fail dominates",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
				{Status: domaintypes.RunRepoStatusFail, Count: 1},
			},
			wantFinish: true,
			wantState:  migsapi.RunStateFailed,
		},
		{
			name: "cancelled when no fail",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
			wantFinish: true,
			wantState:  migsapi.RunStateCancelled,
		},
		{
			name: "running blocks completion",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
			},
			wantFinish: false,
		},
		{
			name:       "empty counts",
			counts:     []store.CountRunReposByStatusRow{},
			wantFinish: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			eval := lifecycle.EvaluateRunCompletionFromRepoCounts(tc.counts)
			if eval.ShouldFinish != tc.wantFinish {
				t.Fatalf("ShouldFinish = %v, want %v", eval.ShouldFinish, tc.wantFinish)
			}
			if tc.wantFinish && eval.RunState != tc.wantState {
				t.Fatalf("RunState = %q, want %q", eval.RunState, tc.wantState)
			}
		})
	}
}
