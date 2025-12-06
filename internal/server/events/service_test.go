package events

// This file contains tests for event storage and persistence behavior.
// SSE streaming tests are in service_stream_test.go.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestStorage_New verifies that the service constructor validates options and
// initializes the service with proper defaults for buffer and history sizes.
func TestStorage_New(t *testing.T) {
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

// TestStorage_ServiceStartStop verifies the service lifecycle, ensuring the service
// can be started and stopped cleanly without errors.
func TestStorage_ServiceStartStop(t *testing.T) {
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

// mockStore is a minimal mock implementation of store.Store for testing.
type mockStore struct {
	store.Querier
	createEventFunc func(ctx context.Context, arg store.CreateEventParams) (store.Event, error)
	createLogFunc   func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
	getJobFunc      func(ctx context.Context, id pgtype.UUID) (store.Job, error)
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

// GetJob returns job metadata for log enrichment.
func (m *mockStore) GetJob(ctx context.Context, id pgtype.UUID) (store.Job, error) {
	if m.getJobFunc != nil {
		return m.getJobFunc(ctx, id)
	}
	return store.Job{}, nil
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

// TestStorage_CreateAndPublishEvent verifies that events are correctly persisted to the
// database and published to the SSE hub. It tests successful creation, database
// errors, and invalid UUID handling.
func TestStorage_CreateAndPublishEvent(t *testing.T) {
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
					JobID:   arg.JobID,
					Time:    arg.Time,
					Level:   arg.Level,
					Message: arg.Message,
					Meta:    arg.Meta,
				}, nil
			},
			params: store.CreateEventParams{
				RunID:   runID,
				JobID:   pgtype.UUID{Valid: false},
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

// TestStorage_LevelNormalization verifies that event log levels
// are normalized to lowercase standard levels (info, warn, error). Unknown or
// empty levels default to "info". This ensures consistent level representation
// in both database storage and SSE streams.
func TestStorage_LevelNormalization(t *testing.T) {
	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

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

			svc, err := New(Options{BufferSize: 4, HistorySize: 8, Store: mock})
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}

			ctx := context.Background()
			params := store.CreateEventParams{
				RunID:   runID,
				JobID:   pgtype.UUID{Valid: false},
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
			streamID := uuid.UUID(params.RunID.Bytes).String()
			snapshot := svc.Hub().Snapshot(streamID)
			if len(snapshot) == 0 {
				t.Fatal("expected SSE event published")
			}
			if snapshot[0].Type != "log" {
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

// TestStorage_CreateAndPublishLog verifies that log chunks are correctly persisted to
// the database and published to the SSE hub. It tests successful creation,
// database errors, and invalid UUID handling for log records.
func TestStorage_CreateAndPublishLog(t *testing.T) {
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
					JobID:     arg.JobID,
					BuildID:   arg.BuildID,
					ChunkNo:   arg.ChunkNo,
					Data:      arg.Data,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
			params: store.CreateLogParams{
				RunID:   runID,
				JobID:   pgtype.UUID{Valid: false},
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

// TestStorage_WithoutStore verifies that the service correctly returns
// errors when attempting to persist events or logs without a configured store.
// This ensures proper error handling for services created without database backing.
func TestStorage_WithoutStore(t *testing.T) {
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

// gzipData compresses data using gzip and returns the compressed bytes.
func gzipData(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err := gzWriter.Write([]byte(data)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

// TestStorage_LogEnrichmentWithJobMetadata verifies that log records are
// enriched with execution context (node_id, job_id, mod_type, step_index)
// when job metadata is available. This ensures SSE clients receive correlated
// log data for diagnostics.
func TestStorage_LogEnrichmentWithJobMetadata(t *testing.T) {
	t.Parallel()

	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}
	jobID := pgtype.UUID{
		Bytes: [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Valid: true,
	}
	nodeID := pgtype.UUID{
		Bytes: [16]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
		Valid: true,
	}

	// Create log data (gzipped, since that's how logs come from nodes).
	logLine := "Build step completed successfully\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		// CreateLog returns the persisted log record.
		createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
			return store.Log{
				ID:        1,
				RunID:     arg.RunID,
				JobID:     arg.JobID,
				BuildID:   arg.BuildID,
				ChunkNo:   arg.ChunkNo,
				Data:      arg.Data,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		// GetJob returns job metadata for enrichment.
		getJobFunc: func(ctx context.Context, id pgtype.UUID) (store.Job, error) {
			if id.Bytes != jobID.Bytes {
				t.Fatalf("GetJob called with unexpected id: got %v, want %v", id, jobID)
			}
			return store.Job{
				ID:        jobID,
				RunID:     runID,
				Name:      "build-step",
				ModType:   "mod",
				StepIndex: 2000,
				NodeID:    nodeID,
			}, nil
		},
	}

	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	params := store.CreateLogParams{
		RunID:   runID,
		JobID:   jobID,
		BuildID: pgtype.UUID{Valid: false},
		ChunkNo: 1,
		Data:    gzippedLog,
	}

	_, err = svc.CreateAndPublishLog(ctx, params)
	if err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	// Verify SSE event contains enriched fields.
	streamID := uuid.UUID(runID.Bytes).String()
	snapshot := svc.Hub().Snapshot(streamID)
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}
	if snapshot[0].Type != "log" {
		t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
	}

	// Unmarshal and verify enriched fields.
	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Verify enriched fields are present.
	wantNodeID := uuid.UUID(nodeID.Bytes).String()
	wantJobID := uuid.UUID(jobID.Bytes).String()
	if rec.NodeID != wantNodeID {
		t.Errorf("node_id: got %q, want %q", rec.NodeID, wantNodeID)
	}
	if rec.JobID != wantJobID {
		t.Errorf("job_id: got %q, want %q", rec.JobID, wantJobID)
	}
	if rec.ModType != "mod" {
		t.Errorf("mod_type: got %q, want %q", rec.ModType, "mod")
	}
	if rec.StepIndex != 2000 {
		t.Errorf("step_index: got %d, want %d", rec.StepIndex, 2000)
	}
}

// TestStorage_LogEnrichmentWithoutJobID verifies that logs without a valid
// job_id are still published without enrichment (graceful degradation).
func TestStorage_LogEnrichmentWithoutJobID(t *testing.T) {
	t.Parallel()

	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

	logLine := "System log without job context\n"
	gzippedLog := gzipData(t, logLine)

	getJobCalled := false
	mock := &mockStore{
		createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
			return store.Log{
				ID:        1,
				RunID:     arg.RunID,
				JobID:     arg.JobID, // Invalid UUID.
				Data:      arg.Data,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		getJobFunc: func(ctx context.Context, id pgtype.UUID) (store.Job, error) {
			getJobCalled = true
			return store.Job{}, nil
		},
	}

	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	params := store.CreateLogParams{
		RunID:   runID,
		JobID:   pgtype.UUID{Valid: false}, // No job ID.
		ChunkNo: 1,
		Data:    gzippedLog,
	}

	_, err = svc.CreateAndPublishLog(ctx, params)
	if err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	// GetJob should NOT be called when JobID is invalid.
	if getJobCalled {
		t.Error("GetJob should not be called when JobID is invalid")
	}

	// Verify log was still published (without enrichment).
	streamID := uuid.UUID(runID.Bytes).String()
	snapshot := svc.Hub().Snapshot(streamID)
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Enriched fields should be empty.
	if rec.NodeID != "" {
		t.Errorf("node_id should be empty, got %q", rec.NodeID)
	}
	if rec.JobID != "" {
		t.Errorf("job_id should be empty, got %q", rec.JobID)
	}
	if rec.ModType != "" {
		t.Errorf("mod_type should be empty, got %q", rec.ModType)
	}
	if rec.StepIndex != 0 {
		t.Errorf("step_index should be 0, got %d", rec.StepIndex)
	}
}

// TestStorage_LogEnrichmentJobLookupFailure verifies that logs are still
// published even when job metadata lookup fails (resilience).
func TestStorage_LogEnrichmentJobLookupFailure(t *testing.T) {
	t.Parallel()

	runID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}
	jobID := pgtype.UUID{
		Bytes: [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Valid: true,
	}

	logLine := "Log with failing job lookup\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
			return store.Log{
				ID:        1,
				RunID:     arg.RunID,
				JobID:     arg.JobID,
				Data:      arg.Data,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		},
		// Simulate job lookup failure.
		getJobFunc: func(ctx context.Context, id pgtype.UUID) (store.Job, error) {
			return store.Job{}, context.DeadlineExceeded
		},
	}

	svc, err := New(Options{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	params := store.CreateLogParams{
		RunID:   runID,
		JobID:   jobID,
		ChunkNo: 1,
		Data:    gzippedLog,
	}

	// Should succeed despite job lookup failure.
	_, err = svc.CreateAndPublishLog(ctx, params)
	if err != nil {
		t.Fatalf("CreateAndPublishLog should succeed despite job lookup failure: %v", err)
	}

	// Verify log was published (without enrichment due to lookup failure).
	streamID := uuid.UUID(runID.Bytes).String()
	snapshot := svc.Hub().Snapshot(streamID)
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Enriched fields should be empty due to lookup failure.
	if rec.NodeID != "" {
		t.Errorf("node_id should be empty after lookup failure, got %q", rec.NodeID)
	}
}
