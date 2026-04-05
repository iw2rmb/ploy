package server

// This file contains tests for event storage and persistence behavior.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestStorage_CreateAndPublishEvent verifies that events are correctly persisted to the
// database and published to the SSE hub. It tests successful creation, database
// errors, and invalid run ID handling.
func TestStorage_CreateAndPublishEvent(t *testing.T) {
	runID := domaintypes.RunID("run-events-123")

	tests := []struct {
		name        string
		storeFunc   func(ctx context.Context, arg store.CreateEventParams) (store.Event, error)
		params      store.CreateEventParams
		wantErr     bool
		checkEvents bool
	}{
		{
			name: "successful event creation and publish",
			storeFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
				return store.Event{
					ID:      1,
					RunID:   arg.RunID,
					JobID:   arg.JobID,
					Time:    arg.Time,
					Level:   arg.Level,
					Message: arg.Message,
					Meta:    arg.Meta,
				}, nil
			},
			params: store.CreateEventParams{
				RunID:   runID,
				JobID:   nil,
				Time:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
				Level:   "info",
				Message: "test event",
				Meta:    []byte("{}"),
			},
			wantErr:     false,
			checkEvents: true,
		},
		{
			name: "database error",
			storeFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
				return store.Event{}, context.DeadlineExceeded
			},
			params: store.CreateEventParams{
				RunID:   runID,
				Level:   "error",
				Message: "failed event",
			},
			wantErr:     true,
			checkEvents: false,
		},
		{
			name: "invalid runID still succeeds DB write (no SSE fanout)",
			storeFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
				return store.Event{
					ID:      2,
					RunID:   arg.RunID,
					Level:   arg.Level,
					Message: arg.Message,
				}, nil
			},
			params: store.CreateEventParams{
				// Whitespace-only run ID is treated as invalid for SSE stream.
				RunID:   domaintypes.RunID("   "),
				Level:   "warn",
				Message: "invalid run id event",
			},
			wantErr:     false,
			checkEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{createEventFunc: tt.storeFunc}
			svc, err := NewEventsService(EventsOptions{
				BufferSize:  4,
				HistorySize: 8,
				Store:       mock,
			})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			event, err := svc.CreateAndPublishEvent(ctx, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if event.ID == 0 {
				t.Fatal("expected non-zero event ID")
			}

			// Check if event was published to hub.
			if tt.checkEvents {
				streamID := strings.TrimSpace(tt.params.RunID.String())
				snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
				if len(snapshot) == 0 {
					t.Fatal("expected event in hub snapshot, got none")
				}
				if snapshot[0].Type != domaintypes.SSEEventStage {
					t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
				}
			}
		})
	}
}

// TestStorage_LevelNormalization verifies that event log levels
// are normalized to lowercase standard levels (info, warn, error). Unknown or
// empty levels default to "info". This ensures consistent level representation
// in both database storage and SSE streams.
func TestStorage_LevelNormalization(t *testing.T) {
	runID := domaintypes.RunID("run-level-normalization")

	type testCase struct {
		inLevel   string
		wantLevel string
	}

	cases := []testCase{
		{inLevel: "INFO", wantLevel: "info"},
		{inLevel: " warn ", wantLevel: "warn"},
		{inLevel: "error", wantLevel: "error"},
		// Unknown or empty map to info
		{inLevel: "warning", wantLevel: "info"},
		{inLevel: "", wantLevel: "info"},
		{inLevel: "verbose", wantLevel: "info"},
	}

	for _, tc := range cases {
		t.Run(tc.inLevel, func(t *testing.T) {
			// Capture the params passed to the store and assert normalization happened
			mock := &mockStore{createEventFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
				if arg.Level != tc.wantLevel {
					t.Fatalf("CreateEvent received level=%q; want %q", arg.Level, tc.wantLevel)
				}
				return store.Event{
					ID:      123,
					RunID:   arg.RunID,
					JobID:   arg.JobID,
					Time:    arg.Time,
					Level:   arg.Level,
					Message: arg.Message,
					Meta:    arg.Meta,
				}, nil
			}}

			svc, err := NewEventsService(EventsOptions{BufferSize: 4, HistorySize: 8, Store: mock})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			params := store.CreateEventParams{
				RunID:   runID,
				JobID:   nil,
				Time:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
				Level:   tc.inLevel,
				Message: "msg",
				Meta:    []byte(`{}`),
			}

			evt, err := svc.CreateAndPublishEvent(ctx, params)
			if err != nil {
				t.Fatalf("CreateAndPublishEvent error: %v", err)
			}

			if evt.Level != tc.wantLevel {
				t.Fatalf("event.Level=%q; want %q", evt.Level, tc.wantLevel)
			}

			// Verify SSE stream used normalized level in LogRecord.Stream
			streamID := strings.TrimSpace(params.RunID.String())
			snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
			if len(snapshot) == 0 {
				t.Fatal("expected SSE event published")
			}
			if snapshot[0].Type != domaintypes.SSEEventStage {
				t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
			}
			var rec logstream.LogRecord
			if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
				t.Fatalf("unmarshal log record: %v", err)
			}
			if rec.Stream != tc.wantLevel {
				t.Fatalf("SSE stream=%q; want %q", rec.Stream, tc.wantLevel)
			}
		})
	}
}
