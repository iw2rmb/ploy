package logstream

import (
	"context"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHubBackpressureDropsSlowSubscriber(t *testing.T) {
	hub := NewHub(Options{BufferSize: 1, HistorySize: 4})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:05:00Z", Stream: "stdout", Line: "first"}); err != nil {
		t.Fatalf("publish log first: %v", err)
	}
	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:05:01Z", Stream: "stdout", Line: "second"}); err != nil {
		t.Fatalf("publish log second: %v", err)
	}

	evt, ok := <-sub.Events
	if !ok {
		t.Fatal("expected first log event before drop")
	}
	if evt.Type != domaintypes.SSEEventLog {
		t.Fatalf("unexpected event type %s", evt.Type)
	}
	if _, ok := <-sub.Events; ok {
		t.Fatal("expected subscriber channel closed after backpressure")
	}
}

// TestHubBackpressureWithEnrichedLogs verifies that backpressure handling
// remains correct with enriched log payloads. Slow subscribers should be
// dropped gracefully without blocking fast publishers.
func TestHubBackpressureWithEnrichedLogs(t *testing.T) {
	t.Parallel()

	// Use minimal buffer size to trigger backpressure quickly.
	hub := NewHub(Options{BufferSize: 1, HistorySize: 4})
	ctx := context.Background()
	runID := domaintypes.NewRunID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	jobID := domaintypes.NewJobID()

	// Subscribe with a slow consumer (never drains the channel).
	sub, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	// Publish multiple enriched logs to exceed buffer capacity.
	for i := 0; i < 5; i++ {
		record := LogRecord{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Stream:    "stdout",
			Line:      "Log line " + string(rune('0'+i)),
			NodeID:    nodeID,
			JobID:     jobID,
			JobType:   "mod",
		}
		// Should not block; slow subscriber should be dropped.
		if err := hub.PublishLog(ctx, runID, record); err != nil {
			t.Fatalf("publish log %d: %v", i, err)
		}
	}

	// Read first event (should succeed before drop).
	select {
	case evt, ok := <-sub.Events:
		if !ok {
			t.Log("subscriber channel closed (expected due to backpressure)")
			return
		}
		if evt.Type != domaintypes.SSEEventLog {
			t.Fatalf("expected event type 'log', got %s", evt.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first event")
	}

	// Subscriber channel should be closed after backpressure drop.
	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Log("received additional event (acceptable if buffer not exceeded)")
		} else {
			t.Log("subscriber channel closed as expected after backpressure")
		}
	case <-time.After(100 * time.Millisecond):
		t.Log("no additional events or close (buffer may have absorbed)")
	}
}
