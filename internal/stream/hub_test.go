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

	if err := hub.PublishStage(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:00:00Z", Stream: "stdout", Line: "stage one"}); err != nil {
		t.Fatalf("publish stage: %v", err)
	}
	if err := hub.PublishStage(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:00:01Z", Stream: "stdout", Line: "stage two"}); err != nil {
		t.Fatalf("publish stage: %v", err)
	}
	if err := hub.PublishStatus(ctx, runID, Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	expect := []domaintypes.SSEEventType{domaintypes.SSEEventStage, domaintypes.SSEEventStage, domaintypes.SSEEventDone}
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
	if len(resumed) != 2 || resumed[0] != domaintypes.SSEEventStage || resumed[1] != domaintypes.SSEEventDone {
		t.Fatalf("unexpected resumed events: %v", resumed)
	}

	if err := hub.PublishStage(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:00:02Z", Stream: "stdout", Line: "late"}); !errors.Is(err, ErrStreamClosed) {
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

// Reused job IDs accept fresh logs without replaying the previous terminal frame.
func TestHub_ResumeJobReopensOnce(t *testing.T) {
	h := NewHub(Options{})
	ctx := context.Background()
	id := domaintypes.NewJobID()
	if err := h.PublishJobStatus(ctx, id, Status{Status: "Error"}); err != nil {
		t.Fatal(err)
	}
	old := h.SnapshotJob(id)
	h.ResumeJob(id, 1)
	if err := h.PublishJobLog(ctx, id, LogRecord{}); err != nil {
		t.Fatal(err)
	}
	h.ResumeJob(id, 1)
	current := h.SnapshotJob(id)
	if len(current) != 1 || current[0].Type != domaintypes.SSEEventLog || current[0].ID <= old[0].ID {
		t.Fatalf("invalid resumed history: %+v", current)
	}
	if err := h.PublishJobStatus(ctx, id, Status{Status: "Success"}); err != nil {
		t.Fatal(err)
	}
	h.ResumeJob(id, 1)
	if err := h.PublishJobLog(ctx, id, LogRecord{}); err != ErrStreamClosed {
		t.Fatalf("same generation reopened completed stream: %v", err)
	}
}
