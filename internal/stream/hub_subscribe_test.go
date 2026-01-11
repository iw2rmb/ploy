package logstream

import (
	"context"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHubConcurrentSubscribersWithResume(t *testing.T) {
	hub := NewHub(Options{BufferSize: 8, HistorySize: 16})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	// Publish initial events before any subscribers join.
	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T14:00:00Z", Stream: "stdout", Line: "event 1"}); err != nil {
		t.Fatalf("publish log 1: %v", err)
	}
	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T14:00:01Z", Stream: "stdout", Line: "event 2"}); err != nil {
		t.Fatalf("publish log 2: %v", err)
	}

	// First subscriber joins from the start (sinceID=0).
	sub1, err := hub.Subscribe(ctx, runID, 0)
	if err != nil {
		t.Fatalf("subscribe sub1: %v", err)
	}
	defer sub1.Cancel()

	// Second subscriber joins with resumption (sinceID=1, should get events 2+).
	sub2, err := hub.Subscribe(ctx, runID, 1)
	if err != nil {
		t.Fatalf("subscribe sub2: %v", err)
	}
	defer sub2.Cancel()

	// Collect initial history for both subscribers.
	var events1, events2 []Event

	// Sub1 should receive events 1 and 2 from history.
	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub1.Events:
			events1 = append(events1, evt)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("sub1: timeout waiting for history event %d", i+1)
		}
	}

	// Sub2 should receive only event 2 from history (since sinceID=1).
	select {
	case evt := <-sub2.Events:
		events2 = append(events2, evt)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2: timeout waiting for history event")
	}

	// Verify initial history reception.
	if len(events1) != 2 {
		t.Fatalf("sub1: expected 2 history events, got %d", len(events1))
	}
	if events1[0].ID != 1 || events1[1].ID != 2 {
		t.Fatalf("sub1: unexpected event IDs: %d, %d", events1[0].ID, events1[1].ID)
	}
	if len(events2) != 1 {
		t.Fatalf("sub2: expected 1 history event, got %d", len(events2))
	}
	if events2[0].ID != 2 {
		t.Fatalf("sub2: unexpected event ID: %d", events2[0].ID)
	}

	// Publish new events that both subscribers should receive concurrently.
	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T14:00:02Z", Stream: "stdout", Line: "event 3"}); err != nil {
		t.Fatalf("publish log 3: %v", err)
	}
	if err := hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T14:00:03Z", Stream: "stdout", Line: "event 4"}); err != nil {
		t.Fatalf("publish log 4: %v", err)
	}

	// Third subscriber joins mid-stream with resumption (sinceID=3, should get event 4+).
	sub3, err := hub.Subscribe(ctx, runID, 3)
	if err != nil {
		t.Fatalf("subscribe sub3: %v", err)
	}
	defer sub3.Cancel()

	// Sub3 should receive event 4 from history.
	var events3 []Event
	select {
	case evt := <-sub3.Events:
		events3 = append(events3, evt)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub3: timeout waiting for history event")
	}
	if len(events3) != 1 || events3[0].ID != 4 {
		t.Fatalf("sub3: expected event ID 4, got %v", events3)
	}

	// Sub1 should receive events 3 and 4.
	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub1.Events:
			events1 = append(events1, evt)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("sub1: timeout waiting for live event %d", i+3)
		}
	}

	// Sub2 should receive events 3 and 4.
	for i := 0; i < 2; i++ {
		select {
		case evt := <-sub2.Events:
			events2 = append(events2, evt)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("sub2: timeout waiting for live event %d", i+3)
		}
	}

	// Verify all subscribers received the new events.
	if len(events1) != 4 {
		t.Fatalf("sub1: expected 4 total events, got %d", len(events1))
	}
	if events1[2].ID != 3 || events1[3].ID != 4 {
		t.Fatalf("sub1: unexpected new event IDs: %d, %d", events1[2].ID, events1[3].ID)
	}
	if len(events2) != 3 {
		t.Fatalf("sub2: expected 3 total events, got %d", len(events2))
	}
	if events2[1].ID != 3 || events2[2].ID != 4 {
		t.Fatalf("sub2: unexpected new event IDs: %d, %d", events2[1].ID, events2[2].ID)
	}

	// Publish final status event.
	if err := hub.PublishStatus(ctx, runID, Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	// All three subscribers should receive the status event.
	select {
	case evt := <-sub1.Events:
		if evt.Type != domaintypes.SSEEventDone || evt.ID != 5 {
			t.Fatalf("sub1: unexpected final event: type=%s id=%d", evt.Type, evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub1: timeout waiting for status event")
	}

	select {
	case evt := <-sub2.Events:
		if evt.Type != domaintypes.SSEEventDone || evt.ID != 5 {
			t.Fatalf("sub2: unexpected final event: type=%s id=%d", evt.Type, evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2: timeout waiting for status event")
	}

	select {
	case evt := <-sub3.Events:
		if evt.Type != domaintypes.SSEEventDone || evt.ID != 5 {
			t.Fatalf("sub3: unexpected final event: type=%s id=%d", evt.Type, evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub3: timeout waiting for status event")
	}

	// All channels should close after the done event.
	for i := 1; i <= 3; i++ {
		var ch <-chan Event
		switch i {
		case 1:
			ch = sub1.Events
		case 2:
			ch = sub2.Events
		case 3:
			ch = sub3.Events
		}
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("sub%d: expected channel closed after done event", i)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("sub%d: timeout waiting for channel close", i)
		}
	}
}

func TestSubscribeClosedStreamFutureSince(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	// Publish a couple of events and close the stream.
	_ = hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T15:00:00Z", Stream: "stdout", Line: "e1"})
	_ = hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T15:00:01Z", Stream: "stdout", Line: "e2"})
	_ = hub.PublishStatus(ctx, runID, Status{Status: "completed"})

	// Subscribe with sinceID far in the future; expect immediate close and no events.
	sub, err := hub.Subscribe(ctx, runID, 999)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Fatal("expected closed channel for future since on closed stream")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for closed channel")
	}
}
