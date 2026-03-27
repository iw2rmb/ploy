package handlers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

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
				{Status: domaintypes.RunRepoStatusQueued, Count: 1},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
				{Status: domaintypes.RunRepoStatusCancelled, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: lifecycle.DerivedStatusCancelled,
		},
		{
			name: "running when any running",
			rows: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusQueued, Count: 2},
				{Status: domaintypes.RunRepoStatusRunning, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: lifecycle.DerivedStatusRunning,
		},
		{
			name: "failed when any fail and none running/cancelled",
			rows: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 2},
				{Status: domaintypes.RunRepoStatusFail, Count: 1},
			},
			wantTotal:         3,
			wantDerivedStatus: lifecycle.DerivedStatusFailed,
		},
		{
			name: "completed when all terminal and no fail/cancelled",
			rows: []store.CountRunReposByStatusRow{
				{Status: domaintypes.RunRepoStatusSuccess, Count: 3},
			},
			wantTotal:         3,
			wantDerivedStatus: lifecycle.DerivedStatusCompleted,
		},
		{
			name:              "pending when queued only",
			rows:              []store.CountRunReposByStatusRow{{Status: domaintypes.RunRepoStatusQueued, Count: 2}},
			wantTotal:         2,
			wantDerivedStatus: lifecycle.DerivedStatusPending,
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
