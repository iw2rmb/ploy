package server

// This file contains tests for log SSE fanout behavior.

import (
	"context"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestStorage_CreateAndPublishLog verifies that logs are correctly published to
// the job-keyed SSE stream for real-time streaming. The log metadata is already
// persisted via blobpersist; this method only handles SSE fanout.
func TestStorage_CreateAndPublishLog(t *testing.T) {
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	logData := []byte("test log line")

	tests := []struct {
		name           string
		log            store.Log
		data           []byte
		checkJobEvents bool
	}{
		{
			name: "successful log publish to job stream",
			log: store.Log{
				ID:        1,
				RunID:     runID,
				JobID:     &jobID,
				ChunkNo:   1,
				DataSize:  int64(len(logData)),
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			},
			data:           logData,
			checkJobEvents: true,
		},
		{
			name: "nil job_id skips SSE fanout",
			log: store.Log{
				ID:       2,
				RunID:    runID,
				JobID:    nil,
				ChunkNo:  2,
				DataSize: int64(len(logData)),
			},
			data:           logData,
			checkJobEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{}
			svc := newTestEventsService(t, mock)

			ctx := context.Background()
			if err := svc.CreateAndPublishLog(ctx, tt.log, tt.data); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkJobEvents {
				snapshot := svc.Hub().SnapshotJob(*tt.log.JobID)
				if len(snapshot) == 0 {
					t.Fatal("expected log event in job hub snapshot, got none")
				}
				if snapshot[0].Type != domaintypes.SSEEventLog {
					t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
				}
			}

			// Verify run stream is empty (logs no longer go to run stream).
			runSnapshot := svc.Hub().Snapshot(runID)
			if len(runSnapshot) != 0 {
				t.Fatalf("expected no events on run stream, got %d", len(runSnapshot))
			}
		})
	}
}
