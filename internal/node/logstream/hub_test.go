package logstream

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubPublishAndResume(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()

	if err := hub.PublishLog(ctx, "job-1", LogRecord{Timestamp: "2025-10-22T12:00:00Z", Stream: "stdout", Line: "line one"}); err != nil {
		t.Fatalf("publish log: %v", err)
	}
	if err := hub.PublishRetention(ctx, "job-1", RetentionHint{Retained: true, TTL: "72h", Bundle: "bafy-logs"}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := hub.PublishStatus(ctx, "job-1", Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	sub, err := hub.Subscribe(ctx, "job-1", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	expect := []string{"log", "retention", "done"}
	received := make([]string, 0, len(expect))
	for evt := range sub.Events {
		received = append(received, evt.Type)
		if evt.Type == "done" {
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

	resume, err := hub.Subscribe(ctx, "job-1", 1)
	if err != nil {
		t.Fatalf("resume subscribe: %v", err)
	}
	defer resume.Cancel()

	resumed := make([]string, 0, 2)
	for evt := range resume.Events {
		resumed = append(resumed, evt.Type)
	}
	if len(resumed) != 2 || resumed[0] != "retention" || resumed[1] != "done" {
		t.Fatalf("unexpected resumed events: %v", resumed)
	}

	if err := hub.PublishLog(ctx, "job-1", LogRecord{Timestamp: "2025-10-22T12:00:01Z", Stream: "stdout", Line: "late"}); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("expected ErrStreamClosed, got %v", err)
	}
}

func TestHubBackpressureDropsSlowSubscriber(t *testing.T) {
	hub := NewHub(Options{BufferSize: 1, HistorySize: 4})
	ctx := context.Background()

	sub, err := hub.Subscribe(ctx, "job-2", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	if err := hub.PublishLog(ctx, "job-2", LogRecord{Timestamp: "2025-10-22T12:05:00Z", Stream: "stdout", Line: "first"}); err != nil {
		t.Fatalf("publish log first: %v", err)
	}
	if err := hub.PublishLog(ctx, "job-2", LogRecord{Timestamp: "2025-10-22T12:05:01Z", Stream: "stdout", Line: "second"}); err != nil {
		t.Fatalf("publish log second: %v", err)
	}

	evt, ok := <-sub.Events
	if !ok {
		t.Fatal("expected first log event before drop")
	}
	if evt.Type != "log" {
		t.Fatalf("unexpected event type %s", evt.Type)
	}
	if _, ok := <-sub.Events; ok {
		t.Fatal("expected subscriber channel closed after backpressure")
	}
}

func TestServeWritesSSEFrames(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = hub.PublishLog(ctx, "job-http", LogRecord{Timestamp: "2025-10-22T12:10:00Z", Stream: "stdout", Line: "hello"})
		_ = hub.PublishStatus(ctx, "job-http", Status{Status: "completed"})
	}()

	req := httptest.NewRequest("GET", "/", nil)
	recorder := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	if err := Serve(recorder, req, hub, "job-http", 0); err != nil {
		t.Fatalf("serve: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "event: done") {
		t.Fatalf("unexpected SSE payload: %s", body)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {
	// ResponseRecorder buffers writes; nothing else required.
}
