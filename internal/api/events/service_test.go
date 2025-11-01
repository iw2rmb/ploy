package events

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/iw2rmb/ploy/internal/node/logstream"
    "github.com/iw2rmb/ploy/internal/store"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jackc/pgx/v5/pgxpool"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name: "valid options with defaults",
			opts: Options{
				BufferSize:  0,
				HistorySize: 0,
			},
			wantErr: false,
		},
		{
			name: "valid options with explicit values",
			opts: Options{
				BufferSize:  32,
				HistorySize: 256,
			},
			wantErr: false,
		},
		{
			name: "negative buffer size",
			opts: Options{
				BufferSize:  -1,
				HistorySize: 256,
			},
			wantErr: true,
		},
		{
			name: "negative history size",
			opts: Options{
				BufferSize:  32,
				HistorySize: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc == nil {
				t.Fatal("expected service, got nil")
			}
			if svc.Hub() == nil {
				t.Fatal("expected hub, got nil")
			}
		})
	}
}

func TestServiceStartStop(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Start the service.
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("failed to start service: %v", err)
	}

	// Stop the service.
	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("failed to stop service: %v", err)
	}
}

func TestServiceHubIntegration(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	hub := svc.Hub()

	// Publish a log event.
	if err := hub.PublishLog(ctx, "test-stream", logstream.LogRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "test log line",
	}); err != nil {
		t.Fatalf("failed to publish log: %v", err)
	}

	// Subscribe to the stream.
	sub, err := hub.Subscribe(ctx, "test-stream", 0)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Cancel()

	// Publish a status event to close the stream.
	if err := hub.PublishStatus(ctx, "test-stream", logstream.Status{
		Status: "completed",
	}); err != nil {
		t.Fatalf("failed to publish status: %v", err)
	}

	// Read events from the subscription.
	events := make([]logstream.Event, 0)
	for evt := range sub.Events {
		events = append(events, evt)
		if evt.Type == "done" {
			break
		}
	}

	// Verify we received both events.
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "log" {
		t.Fatalf("expected first event type 'log', got %s", events[0].Type)
	}
	if events[1].Type != "done" {
		t.Fatalf("expected second event type 'done', got %s", events[1].Type)
	}
}

// mockStore is a minimal mock implementation of store.Store for testing.
type mockStore struct {
	store.Querier
	createEventFunc func(ctx context.Context, arg store.CreateEventParams) (store.Event, error)
	createLogFunc   func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
}

func (m *mockStore) CreateEvent(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
	if m.createEventFunc != nil {
		return m.createEventFunc(ctx, arg)
	}
	return store.Event{}, nil
}

func (m *mockStore) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	if m.createLogFunc != nil {
		return m.createLogFunc(ctx, arg)
	}
	return store.Log{}, nil
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

func TestCreateAndPublishEvent(t *testing.T) {
	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

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
					StageID: arg.StageID,
					Time:    arg.Time,
					Level:   arg.Level,
					Message: arg.Message,
					Meta:    arg.Meta,
				}, nil
			},
			params: store.CreateEventParams{
				RunID:   runID,
				StageID: pgtype.UUID{Valid: false},
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
			name: "invalid runID still succeeds DB write",
			storeFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
				return store.Event{
					ID:      2,
					RunID:   arg.RunID,
					Level:   arg.Level,
					Message: arg.Message,
				}, nil
			},
			params: store.CreateEventParams{
				RunID:   pgtype.UUID{Valid: false},
				Level:   "warn",
				Message: "invalid uuid event",
			},
			wantErr:     false,
			checkEvents: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{createEventFunc: tt.storeFunc}
			svc, err := New(Options{
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
                streamID := ""
                if tt.params.RunID.Valid {
                    streamID = uuid.UUID(tt.params.RunID.Bytes).String()
                }
                snapshot := svc.Hub().Snapshot(streamID)
                if len(snapshot) == 0 {
                    t.Fatal("expected event in hub snapshot, got none")
                }
                if snapshot[0].Type != "log" {
                    t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
                }
            }
		})
	}
}

func TestCreateAndPublishLog(t *testing.T) {
	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

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
					StageID:   arg.StageID,
					BuildID:   arg.BuildID,
					ChunkNo:   arg.ChunkNo,
					Data:      arg.Data,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
			params: store.CreateLogParams{
				RunID:   runID,
				StageID: pgtype.UUID{Valid: false},
				BuildID: pgtype.UUID{Valid: false},
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
			name: "invalid runID still succeeds DB write",
			storeFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
				return store.Log{
					ID:      3,
					RunID:   arg.RunID,
					ChunkNo: arg.ChunkNo,
					Data:    arg.Data,
				}, nil
			},
			params: store.CreateLogParams{
				RunID:   pgtype.UUID{Valid: false},
				ChunkNo: 3,
				Data:    []byte("invalid uuid log"),
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
                streamID := ""
                if tt.params.RunID.Valid {
                    streamID = uuid.UUID(tt.params.RunID.Bytes).String()
                }
                snapshot := svc.Hub().Snapshot(streamID)
                if len(snapshot) == 0 {
                    t.Fatal("expected log event in hub snapshot, got none")
                }
                if snapshot[0].Type != "log" {
                    t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
                }
            }
		})
	}
}

func TestCreateAndPublishWithoutStore(t *testing.T) {
	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
		Store:       nil,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Test CreateAndPublishEvent without store.
	_, err = svc.CreateAndPublishEvent(ctx, store.CreateEventParams{})
	if err == nil {
		t.Fatal("expected error when store not configured, got nil")
	}

	// Test CreateAndPublishLog without store.
	_, err = svc.CreateAndPublishLog(ctx, store.CreateLogParams{})
	if err == nil {
		t.Fatal("expected error when store not configured, got nil")
	}
}
