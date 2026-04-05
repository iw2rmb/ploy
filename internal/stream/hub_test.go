package logstream

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHubPublishAndResume(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:00:00Z", Stream: "stdout", Line: "line one"}); err != nil {
		t.Fatalf("publish log: %v", err)
	}
	if err := hub.PublishRetention(ctx, runID, RetentionHint{Retained: true, TTL: "72h", Bundle: "bafy-logs"}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := hub.PublishStatus(ctx, runID, Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	expect := []domaintypes.SSEEventType{domaintypes.SSEEventLog, domaintypes.SSEEventRetention, domaintypes.SSEEventDone}
	received := make([]domaintypes.SSEEventType, 0, len(expect))
	for evt := range sub.Events {
		received = append(received, evt.Type)
		if evt.Type == domaintypes.SSEEventDone {
			break
		}
	}
	if len(received) != len(expect) {
		t.Fatalf("expected %d events, got %d", len(expect), len(received))
	}
	for i, typ := range expect {
		if received[i] != typ {
			t.Fatalf("expected event %s at position %d, got %s", typ, i, received[i])
		}
	}

	resume, err := hub.Subscribe(ctx, runID, 1)
	if err != nil {
		t.Fatalf("resume subscribe: %v", err)
	}
	defer resume.Cancel()

	resumed := make([]domaintypes.SSEEventType, 0, 2)
	for evt := range resume.Events {
		resumed = append(resumed, evt.Type)
	}
	if len(resumed) != 2 || resumed[0] != domaintypes.SSEEventRetention || resumed[1] != domaintypes.SSEEventDone {
		t.Fatalf("unexpected resumed events: %v", resumed)
	}

	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:00:01Z", Stream: "stdout", Line: "late"}); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("expected ErrStreamClosed, got %v", err)
	}
}

func TestHubSubscribeRejectsNegativeEventID(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	_, err := hub.Subscribe(ctx, runID, domaintypes.EventID(-1))
	if err == nil {
		t.Fatal("expected error for negative sinceID, got nil")
	}
}

func TestHubJobStreamPublishAndResume(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	jobID := domaintypes.NewJobID()

	if err := hub.PublishJobLog(ctx, jobID, LogRecord{Timestamp: "2025-10-22T12:00:00Z", Stream: "stdout", Line: "line one"}); err != nil {
		t.Fatalf("publish job log: %v", err)
	}
	if err := hub.PublishJobLog(ctx, jobID, LogRecord{Timestamp: "2025-10-22T12:00:01Z", Stream: "stdout", Line: "line two"}); err != nil {
		t.Fatalf("publish job log: %v", err)
	}
	if err := hub.PublishJobStatus(ctx, jobID, Status{Status: "Success"}); err != nil {
		t.Fatalf("publish job status: %v", err)
	}

	sub, err := hub.SubscribeJob(ctx, jobID, 0)
	if err != nil {
		t.Fatalf("subscribe job: %v", err)
	}
	defer sub.Cancel()

	expect := []domaintypes.SSEEventType{domaintypes.SSEEventLog, domaintypes.SSEEventLog, domaintypes.SSEEventDone}
	received := make([]domaintypes.SSEEventType, 0, len(expect))
	for evt := range sub.Events {
		received = append(received, evt.Type)
	}
	if len(received) != len(expect) {
		t.Fatalf("expected %d events, got %d", len(expect), len(received))
	}
	for i, typ := range expect {
		if received[i] != typ {
			t.Fatalf("expected event %s at position %d, got %s", typ, i, received[i])
		}
	}

	// Resume from id=1 should skip first log.
	resume, err := hub.SubscribeJob(ctx, jobID, 1)
	if err != nil {
		t.Fatalf("resume subscribe: %v", err)
	}
	defer resume.Cancel()

	resumed := make([]domaintypes.SSEEventType, 0, 2)
	for evt := range resume.Events {
		resumed = append(resumed, evt.Type)
	}
	if len(resumed) != 2 || resumed[0] != domaintypes.SSEEventLog || resumed[1] != domaintypes.SSEEventDone {
		t.Fatalf("unexpected resumed events: %v", resumed)
	}

	// Publishing after done should fail.
	if err := hub.PublishJobLog(ctx, jobID, LogRecord{Line: "late"}); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("expected ErrStreamClosed, got %v", err)
	}
}

func TestHubJobStreamIsolatedFromRunStream(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	// Publish a stage event to the run stream.
	if err := hub.PublishStage(ctx, runID, LogRecord{Line: "stage"}); err != nil {
		t.Fatalf("publish stage: %v", err)
	}

	// Publish a log event to the job stream.
	if err := hub.PublishJobLog(ctx, jobID, LogRecord{Line: "log"}); err != nil {
		t.Fatalf("publish job log: %v", err)
	}

	// Run stream should have only the stage event.
	runSnap := hub.Snapshot(runID)
	if len(runSnap) != 1 || runSnap[0].Type != domaintypes.SSEEventStage {
		t.Fatalf("run stream: expected 1 stage event, got %d events", len(runSnap))
	}

	// Job stream should have only the log event.
	jobSnap := hub.SnapshotJob(jobID)
	if len(jobSnap) != 1 || jobSnap[0].Type != domaintypes.SSEEventLog {
		t.Fatalf("job stream: expected 1 log event, got %d events", len(jobSnap))
	}
}

func TestHubCloseAllIncludesJobStreams(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()

	_ = hub.PublishStage(ctx, runID, LogRecord{Line: "stage"})
	_ = hub.PublishJobLog(ctx, jobID, LogRecord{Line: "log"})

	// Subscribe to both streams before CloseAll.
	runSub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe run: %v", err)
	}
	jobSub, err := hub.SubscribeJob(ctx, jobID, 0)
	if err != nil {
		t.Fatalf("subscribe job: %v", err)
	}

	hub.CloseAll()

	// Both subscriber channels should be closed.
	drained := 0
	for range runSub.Events {
		drained++
	}
	for range jobSub.Events {
		drained++
	}
	// Should have drained the history events and then channels closed.
	if drained != 2 {
		t.Fatalf("expected 2 drained events (1 run + 1 job), got %d", drained)
	}
}
