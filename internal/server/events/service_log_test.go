package events

// This file contains tests for log storage and persistence behavior.

import (
	"context"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestStorage_CreateAndPublishLog verifies that log chunks are correctly persisted to
// the database and published to the SSE hub. It tests successful creation,
// database errors, and invalid run ID handling for log records.
func TestStorage_CreateAndPublishLog(t *testing.T) {
	runID := domaintypes.RunID("run-logs-123")

	tests := []struct {
		name        string
		storeFunc   func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
		params      store.CreateLogParams
		wantErr     bool
		checkEvents bool
	}{
		{
			name: "successful log creation and publish",
			storeFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
				return store.Log{
					ID:        1,
					RunID:     arg.RunID,
					JobID:     arg.JobID,
					ChunkNo:   arg.ChunkNo,
					Data:      arg.Data,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
			params: store.CreateLogParams{
				RunID:   runID,
				JobID:   nil,
				ChunkNo: 1,
				Data:    []byte("test log line"),
			},
			wantErr:     false,
			checkEvents: true,
		},
		{
			name: "database error",
			storeFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
				return store.Log{}, context.DeadlineExceeded
			},
			params: store.CreateLogParams{
				RunID:   runID,
				ChunkNo: 2,
				Data:    []byte("failed log"),
			},
			wantErr:     true,
			checkEvents: false,
		},
		{
			name: "invalid runID still succeeds DB write (no SSE fanout)",
			storeFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
				return store.Log{
					ID:      3,
					RunID:   arg.RunID,
					ChunkNo: arg.ChunkNo,
					Data:    arg.Data,
				}, nil
			},
			params: store.CreateLogParams{
				RunID:   domaintypes.RunID("   "),
				ChunkNo: 3,
				Data:    []byte("invalid run id log"),
			},
			wantErr:     false,
			checkEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{createLogFunc: tt.storeFunc}
			svc, err := New(Options{
				BufferSize:  4,
				HistorySize: 8,
				Store:       mock,
			})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			log, err := svc.CreateAndPublishLog(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if log.ID == 0 {
				t.Fatal("expected non-zero log ID")
			}

			// Check if log was published to hub.
			if tt.checkEvents {
				streamID := strings.TrimSpace(tt.params.RunID.String())
				snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
				if len(snapshot) == 0 {
					t.Fatal("expected log event in hub snapshot, got none")
				}
				if snapshot[0].Type != domaintypes.SSEEventLog {
					t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
				}
			}
		})
	}
}
