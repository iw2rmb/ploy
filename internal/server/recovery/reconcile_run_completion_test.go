package recovery

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestReconcileRun_EvaluateRunCompletionFromRepoCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		counts     []store.CountRunReposByStatusRow
		wantFinish bool
		wantState  modsapi.RunState
	}{
		{
			name: "all success",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 2},
			},
			wantFinish: true,
			wantState:  modsapi.RunStateSucceeded,
		},
		{
			name: "fail dominates",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
				{Status: domaintypes.RunRepoStatusFail, Count: 1},
			},
			wantFinish: true,
			wantState:  modsapi.RunStateFailed,
		},
		{
			name: "cancelled when no fail",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
			wantFinish: true,
			wantState:  modsapi.RunStateCancelled,
		},
		{
			name: "running blocks completion",
			counts: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
			},
			wantFinish: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			eval := EvaluateRunCompletionFromRepoCounts(tc.counts)
			if eval.ShouldFinish != tc.wantFinish {
				t.Fatalf("ShouldFinish = %v, want %v", eval.ShouldFinish, tc.wantFinish)
			}
			if tc.wantFinish && eval.RunState != tc.wantState {
				t.Fatalf("RunState = %q, want %q", eval.RunState, tc.wantState)
			}
		})
	}
}
