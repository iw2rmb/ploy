package server

// This file contains tests for log SSE fanout behavior.

import (
	"context"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestStorage_CreateAndPublishLog verifies that logs are correctly published to
// the SSE hub for real-time streaming. The log metadata is already persisted via
// blobpersist; this method only handles SSE fanout with optional job enrichment.
func TestStorage_CreateAndPublishLog(t *testing.T) {
	runID := domaintypes.RunID("run-logs-123")
	jobID := domaintypes.NewJobID()
	logData := []byte("test log line")

	tests := []struct {
		name        string
		log         store.Log
		data        []byte
		checkEvents bool
	}{
		{
			name: "successful log publish",
			log: store.Log{
				ID:        1,
				RunID:     runID,
				JobID:     &jobID,
				ChunkNo:   1,
				DataSize:  int64(len(logData)),
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			},
			data:        logData,
			checkEvents: true,
		},
		{
			name: "invalid runID still succeeds (no SSE fanout)",
			log: store.Log{
				ID:       3,
				RunID:    domaintypes.RunID("   "),
				ChunkNo:  3,
				DataSize: int64(len(logData)),
			},
			data:        logData,
			checkEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			svc, err := NewEventsService(EventsOptions{
				BufferSize:  4,
				HistorySize: 8,
				Store:       mock,
			})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			err = svc.CreateAndPublishLog(ctx, tt.log, tt.data)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check if log was published to hub.
			if tt.checkEvents {
				streamID := strings.TrimSpace(tt.log.RunID.String())
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
