package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	logstream "github.com/iw2rmb/ploy/internal/stream"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestFanout_LogsGoToJobStreamOnly verifies that CreateAndPublishLog publishes
// to the job-keyed stream and nothing reaches the run-keyed stream.
func TestFanout_LogsGoToJobStreamOnly(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

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
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     &jobID,
		ChunkNo:   1,
		DataSize:  10,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	if err := svc.CreateAndPublishLog(ctx, logRow, gzipData(t, "hello\n")); err != nil {
		t.Fatalf("CreateAndPublishLog: %v", err)
	}

	// Job stream must have the log event.
	jobSnap := svc.Hub().SnapshotJob(jobID)
	if len(jobSnap) == 0 {
		t.Fatal("expected log event on job stream")
	}
	if jobSnap[0].Type != domaintypes.SSEEventLog {
		t.Fatalf("expected SSEEventLog, got %s", jobSnap[0].Type)
	}

	// Run stream must be empty.
	runSnap := svc.Hub().Snapshot(runID)
	if len(runSnap) != 0 {
		t.Fatalf("expected no events on run stream, got %d", len(runSnap))
	}
}

// TestFanout_EventsGoToRunStreamAsStage verifies that CreateAndPublishEvent
// publishes to the run-keyed stream as stage events.
func TestFanout_EventsGoToRunStreamAsStage(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	mock := &mockStore{
		createEventFunc: func(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
			return store.Event{
				ID:      1,
				RunID:   arg.RunID,
				Level:   arg.Level,
				Message: arg.Message,
				Time:    arg.Time,
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
	_, err = svc.CreateAndPublishEvent(ctx, store.CreateEventParams{
		RunID:   runID,
		Level:   "info",
		Message: "step started",
		Time:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAndPublishEvent: %v", err)
	}

	runSnap := svc.Hub().Snapshot(runID)
	if len(runSnap) == 0 {
		t.Fatal("expected stage event on run stream")
	}
	if runSnap[0].Type != domaintypes.SSEEventStage {
		t.Fatalf("expected SSEEventStage, got %s", runSnap[0].Type)
	}

	var rec logstream.LogRecord
	if err := json.Unmarshal(runSnap[0].Data, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.Line != "step started" {
		t.Fatalf("expected line 'step started', got %q", rec.Line)
	}
}

// TestFanout_NilJobIDSkipsFanout verifies that logs without a job_id
// are silently skipped (no SSE fanout).
func TestFanout_NilJobIDSkipsFanout(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()

	svc, err := NewEventsService(EventsOptions{BufferSize: 4, HistorySize: 8})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	logRow := store.Log{
		ID:        1,
		RunID:     runID,
		JobID:     nil,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	if err := svc.CreateAndPublishLog(ctx, logRow, []byte("data")); err != nil {
		t.Fatalf("CreateAndPublishLog: %v", err)
	}

	// Neither stream should have events.
	if snap := svc.Hub().Snapshot(runID); len(snap) != 0 {
		t.Fatalf("expected empty run stream, got %d events", len(snap))
	}
}

// TestFanout_JobDoneSentinelClosesStream verifies that PublishJobDone
// emits a done event and closes the job stream.
func TestFanout_JobDoneSentinelClosesStream(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()

	svc, err := NewEventsService(EventsOptions{BufferSize: 4, HistorySize: 8})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Publish a log first so the stream exists.
	_ = svc.Hub().PublishJobLog(ctx, jobID, logstream.LogRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Stream:    "stdout",
		Line:      "output",
	})

	// Publish done.
	if err := svc.PublishJobDone(ctx, jobID, "Success"); err != nil {
		t.Fatalf("PublishJobDone: %v", err)
	}

	// Subscribing after done should get history including done, then channel close.
	sub, err := svc.Hub().SubscribeJob(ctx, jobID, 0)
	if err != nil {
		t.Fatalf("SubscribeJob: %v", err)
	}
	defer sub.Cancel()

	var types []domaintypes.SSEEventType
	for evt := range sub.Events {
		types = append(types, evt.Type)
	}

	if len(types) != 2 {
		t.Fatalf("expected 2 events (log, done), got %d: %v", len(types), types)
	}
	if types[0] != domaintypes.SSEEventLog {
		t.Errorf("event[0]: expected log, got %s", types[0])
	}
	if types[1] != domaintypes.SSEEventDone {
		t.Errorf("event[1]: expected done, got %s", types[1])
	}

	// Publishing after done should fail.
	err = svc.Hub().PublishJobLog(ctx, jobID, logstream.LogRecord{Line: "late"})
	if err != logstream.ErrStreamClosed {
		t.Fatalf("expected ErrStreamClosed, got %v", err)
	}
}
