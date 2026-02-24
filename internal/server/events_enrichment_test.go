package server

// This file contains tests for log enrichment with job metadata.

import (
	"bytes"
	"compress/gzip"
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
// enriched with execution context (node_id, job_id, job_type, next_id)
// when job metadata is available. This ensures SSE clients receive correlated
// log data for diagnostics.
func TestStorage_LogEnrichmentWithJobMetadata(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

	// Create log data (gzipped, since that's how logs come from nodes).
	logLine := "Build step completed successfully\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		// GetJob returns job metadata for enrichment.
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			if id != jobID {
				t.Fatalf("GetJob called with unexpected id: got %v, want %v", id, jobID)
			}
			return store.Job{
				ID:      jobID,
				RunID:   runID,
				Name:    "build-step",
				JobType: "mig",
				Meta:    []byte(`{"next_id":2000}`),
				NodeID:  &nodeID,
			}, nil
		},
	}

	svc, err := NewEventsService(EventsOptions{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	// CreateAndPublishLog now takes the already-persisted log metadata and data bytes.
	// The log is already persisted via blobpersist; this method only handles SSE fanout.
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     &jobID,
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	err = svc.CreateAndPublishLog(ctx, logRow, gzippedLog)
	if err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	// Verify SSE event contains enriched fields.
	streamID := strings.TrimSpace(runID.String())
	snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}
	if snapshot[0].Type != domaintypes.SSEEventLog {
		t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
	}

	// Unmarshal and verify enriched fields.
	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Verify enriched fields are present.
	// Compare as domain types (LogRecord fields are now NodeID/JobID domain types).
	if rec.NodeID != nodeID {
		t.Errorf("node_id: got %q, want %q", rec.NodeID, nodeID)
	}
	if rec.JobID != jobID {
		t.Errorf("job_id: got %q, want %q", rec.JobID, jobID)
	}
	if rec.JobType != "mig" {
		t.Errorf("job_type: got %q, want %q", rec.JobType, "mig")
	}
}

// TestLogRecord_LogEnrichmentPreservesTypedFields is a contract test that runs under
// `-run TestLogRecord` and ensures the server publish path emits the canonical
// logstream.LogRecord shape with typed enriched fields without truncation.
func TestLogRecord_LogEnrichmentPreservesTypedFields(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

	logLine := "hello\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			return store.Job{
				ID:      jobID,
				RunID:   runID,
				Name:    "pre-gate",
				JobType: "pre_gate",
				Meta:    []byte(`{"next_id":2000}`),
				NodeID:  &nodeID,
			}, nil
		},
	}

	svc, err := NewEventsService(EventsOptions{BufferSize: 4, HistorySize: 8, Store: mock})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     &jobID,
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	err = svc.CreateAndPublishLog(ctx, logRow, gzippedLog)
	if err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	snapshot := svc.Hub().Snapshot(domaintypes.RunID(strings.TrimSpace(runID.String())))
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}
	if snapshot[0].Type != domaintypes.SSEEventLog {
		t.Fatalf("expected event type 'log', got %s", snapshot[0].Type)
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}
	if rec.NodeID != nodeID {
		t.Errorf("node_id: got %q, want %q", rec.NodeID, nodeID)
	}
	if rec.JobID != jobID {
		t.Errorf("job_id: got %q, want %q", rec.JobID, jobID)
	}
	if rec.JobType != domaintypes.JobTypePreGate {
		t.Errorf("job_type: got %q, want %q", rec.JobType, domaintypes.JobTypePreGate)
	}
}

// TestStorage_LogEnrichmentWithoutJobID verifies that logs without a valid
// job_id are still published without enrichment (graceful degradation).
func TestStorage_LogEnrichmentWithoutJobID(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	logLine := "System log without job context\n"
	gzippedLog := gzipData(t, logLine)

	getJobCalled := false
	mock := &mockStore{
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			getJobCalled = true
			return store.Job{}, nil
		},
	}

	svc, err := NewEventsService(EventsOptions{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	// Log without job ID (log already persisted via blobpersist).
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     nil, // No job ID.
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	err = svc.CreateAndPublishLog(ctx, logRow, gzippedLog)
	if err != nil {
		t.Fatalf("CreateAndPublishLog failed: %v", err)
	}

	// GetJob should NOT be called when JobID is invalid.
	if getJobCalled {
		t.Error("GetJob should not be called when JobID is invalid")
	}

	// Verify log was still published (without enrichment).
	streamID := strings.TrimSpace(runID.String())
	snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Enriched fields should be empty.
	if !rec.NodeID.IsZero() {
		t.Errorf("node_id should be empty, got %q", rec.NodeID)
	}
	if !rec.JobID.IsZero() {
		t.Errorf("job_id should be empty, got %q", rec.JobID)
	}
	if !rec.JobType.IsZero() {
		t.Errorf("job_type should be empty, got %q", rec.JobType)
	}
}

// TestStorage_LogEnrichmentJobLookupFailure verifies that logs are still
// published even when job metadata lookup fails (resilience).
func TestStorage_LogEnrichmentJobLookupFailure(t *testing.T) {
	t.Parallel()

	runID := domaintypes.RunID("run-log-lookup-fail")
	jobID := domaintypes.JobID("job-log-lookup-fail")

	logLine := "Log with failing job lookup\n"
	gzippedLog := gzipData(t, logLine)

	mock := &mockStore{
		// Simulate job lookup failure.
		getJobFunc: func(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
			return store.Job{}, context.DeadlineExceeded
		},
	}

	svc, err := NewEventsService(EventsOptions{
		BufferSize:  4,
		HistorySize: 8,
		Store:       mock,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     &jobID,
		ChunkNo:   1,
		DataSize:  int64(len(gzippedLog)),
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	// Should succeed despite job lookup failure.
	err = svc.CreateAndPublishLog(ctx, logRow, gzippedLog)
	if err != nil {
		t.Fatalf("CreateAndPublishLog should succeed despite job lookup failure: %v", err)
	}

	// Verify log was published (without enrichment due to lookup failure).
	streamID := strings.TrimSpace(runID.String())
	snapshot := svc.Hub().Snapshot(domaintypes.RunID(streamID))
	if len(snapshot) == 0 {
		t.Fatal("expected log event in hub snapshot, got none")
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(snapshot[0].Data, &rec); err != nil {
		t.Fatalf("failed to unmarshal log record: %v", err)
	}

	// Enriched fields should be empty due to lookup failure.
	if !rec.NodeID.IsZero() {
		t.Errorf("node_id should be empty after lookup failure, got %q", rec.NodeID)
	}
}
