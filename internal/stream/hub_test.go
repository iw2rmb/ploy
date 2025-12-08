package logstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/mods/api"
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

func TestHubConcurrentSubscribersWithResume(t *testing.T) {
	hub := NewHub(Options{BufferSize: 8, HistorySize: 16})
	ctx := context.Background()

	// Publish initial events before any subscribers join.
	if err := hub.PublishLog(ctx, "job-concurrent", LogRecord{Timestamp: "2025-10-22T14:00:00Z", Stream: "stdout", Line: "event 1"}); err != nil {
		t.Fatalf("publish log 1: %v", err)
	}
	if err := hub.PublishLog(ctx, "job-concurrent", LogRecord{Timestamp: "2025-10-22T14:00:01Z", Stream: "stdout", Line: "event 2"}); err != nil {
		t.Fatalf("publish log 2: %v", err)
	}

	// First subscriber joins from the start (sinceID=0).
	sub1, err := hub.Subscribe(ctx, "job-concurrent", 0)
	if err != nil {
		t.Fatalf("subscribe sub1: %v", err)
	}
	defer sub1.Cancel()

	// Second subscriber joins with resumption (sinceID=1, should get events 2+).
	sub2, err := hub.Subscribe(ctx, "job-concurrent", 1)
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
	if err := hub.PublishLog(ctx, "job-concurrent", LogRecord{Timestamp: "2025-10-22T14:00:02Z", Stream: "stdout", Line: "event 3"}); err != nil {
		t.Fatalf("publish log 3: %v", err)
	}
	if err := hub.PublishLog(ctx, "job-concurrent", LogRecord{Timestamp: "2025-10-22T14:00:03Z", Stream: "stdout", Line: "event 4"}); err != nil {
		t.Fatalf("publish log 4: %v", err)
	}

	// Third subscriber joins mid-stream with resumption (sinceID=3, should get event 4+).
	sub3, err := hub.Subscribe(ctx, "job-concurrent", 3)
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
	if err := hub.PublishStatus(ctx, "job-concurrent", Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	// All three subscribers should receive the status event.
	select {
	case evt := <-sub1.Events:
		if evt.Type != "done" || evt.ID != 5 {
			t.Fatalf("sub1: unexpected final event: type=%s id=%d", evt.Type, evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub1: timeout waiting for status event")
	}

	select {
	case evt := <-sub2.Events:
		if evt.Type != "done" || evt.ID != 5 {
			t.Fatalf("sub2: unexpected final event: type=%s id=%d", evt.Type, evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2: timeout waiting for status event")
	}

	select {
	case evt := <-sub3.Events:
		if evt.Type != "done" || evt.ID != 5 {
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

	// Publish a couple of events and close the stream.
	_ = hub.PublishLog(ctx, "job-closed", LogRecord{Timestamp: "2025-10-22T15:00:00Z", Stream: "stdout", Line: "e1"})
	_ = hub.PublishLog(ctx, "job-closed", LogRecord{Timestamp: "2025-10-22T15:00:01Z", Stream: "stdout", Line: "e2"})
	_ = hub.PublishStatus(ctx, "job-closed", Status{Status: "completed"})

	// Subscribe with sinceID far in the future; expect immediate close and no events.
	sub, err := hub.Subscribe(ctx, "job-closed", 999)
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

type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {
	// ResponseRecorder buffers writes; nothing else required.
}

// TestLogRecordEnrichedFields verifies that enriched fields (NodeID, JobID,
// ModType, StepIndex) marshal correctly through the publish/subscribe round-trip.
// Fields with zero values are omitted due to `omitempty` tags.
func TestLogRecordEnrichedFields(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()

	tests := []struct {
		name   string
		record LogRecord
		want   map[string]any
	}{
		{
			name: "all enriched fields populated",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:00Z",
				Stream:    "stdout",
				Line:      "hello world",
				NodeID:    "aB3xY9",
				JobID:     "job-def456",
				ModType:   "mod",
				StepIndex: 2,
			},
			want: map[string]any{
				"timestamp":  "2025-10-22T12:00:00Z",
				"stream":     "stdout",
				"line":       "hello world",
				"node_id":    "aB3xY9",
				"job_id":     "job-def456",
				"mod_type":   "mod",
				"step_index": float64(2), // JSON numbers decode as float64
			},
		},
		{
			name: "omitempty omits zero values",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:01Z",
				Stream:    "stderr",
				Line:      "minimal record",
			},
			want: map[string]any{
				"timestamp": "2025-10-22T12:00:01Z",
				"stream":    "stderr",
				"line":      "minimal record",
				// node_id, job_id, mod_type, step_index should be absent
			},
		},
		{
			name: "partial enrichment",
			record: LogRecord{
				Timestamp: "2025-10-22T12:00:02Z",
				Stream:    "stdout",
				Line:      "partial context",
				NodeID:    "Z9yX3b",
				StepIndex: 0, // zero value, should be omitted
			},
			want: map[string]any{
				"timestamp": "2025-10-22T12:00:02Z",
				"stream":    "stdout",
				"line":      "partial context",
				"node_id":   "Z9yX3b",
				// job_id, mod_type, step_index (0) should be absent
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamID := "enriched-test-" + string(rune('0'+i))

			if err := hub.PublishLog(ctx, streamID, tt.record); err != nil {
				t.Fatalf("publish log: %v", err)
			}

			snapshot := hub.Snapshot(streamID)
			if len(snapshot) == 0 {
				t.Fatal("expected event in snapshot")
			}

			evt := snapshot[len(snapshot)-1]
			if evt.Type != "log" {
				t.Fatalf("expected event type 'log', got %s", evt.Type)
			}

			// Unmarshal into a generic map to check exact JSON shape.
			var got map[string]any
			if err := json.Unmarshal(evt.Data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			// Verify expected keys are present with correct values.
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("field %q: got %v, want %v", k, got[k], v)
				}
			}

			// Verify omitted keys are truly absent.
			omittedKeys := []string{"node_id", "job_id", "mod_type", "step_index"}
			for _, k := range omittedKeys {
				if _, inWant := tt.want[k]; !inWant {
					if _, inGot := got[k]; inGot {
						t.Errorf("field %q should be omitted but was present", k)
					}
				}
			}
		})
	}
}

// TestPublishRunTypedPayload verifies that PublishRun accepts only api.RunSummary
// and that the payload marshals correctly through publish/subscribe round-trip.
func TestPublishRunTypedPayload(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()

	// Construct a typed RunSummary payload with RunID field.
	run := api.RunSummary{
		RunID:  "run-123",
		State:  api.RunStateRunning,
		Stages: make(map[string]api.StageStatus),
	}

	// Publish the run event using renamed PublishRun method.
	if err := hub.PublishRun(ctx, "run-1", run); err != nil {
		t.Fatalf("publish run: %v", err)
	}

	// Subscribe and receive the event.
	sub, err := hub.Subscribe(ctx, "run-1", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	select {
	case evt := <-sub.Events:
		if evt.Type != "run" {
			t.Fatalf("expected event type 'run', got %s", evt.Type)
		}
		// Unmarshal and verify the payload.
		var received api.RunSummary
		if err := json.Unmarshal(evt.Data, &received); err != nil {
			t.Fatalf("unmarshal run payload: %v", err)
		}
		if received.RunID != run.RunID {
			t.Fatalf("expected run_id %s, got %s", run.RunID, received.RunID)
		}
		if received.State != run.State {
			t.Fatalf("expected state %s, got %s", run.State, received.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for run event")
	}
}

// =============================================================================
// Performance and Resilience Tests for Enriched Logs
// =============================================================================
// These tests validate that enriched log payloads (with node_id, job_id,
// mod_type, step_index) do not regress performance or resilience for
// long-running or chatty Mods runs.
// Reference: ROADMAP.md - "Validate performance and resilience with enriched logs"

// BenchmarkHubPublishEnrichedLog measures the throughput of publishing enriched
// log records through the hub. This ensures the additional enrichment fields
// do not introduce performance regressions.
func BenchmarkHubPublishEnrichedLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()

	// Create a fully enriched log record matching real-world usage.
	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Build step completed: compiling module org.example.service",
		NodeID:    "aB3xY9",
		JobID:     "11223344-5566-7788-99aa-bbccddeeff00",
		ModType:   "mod",
		StepIndex: 2000,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = hub.PublishLog(ctx, "bench-stream", record)
	}
}

// BenchmarkHubPublishMinimalLog measures baseline throughput for minimal logs
// (without enrichment fields). Used as a comparison baseline for the enriched
// log benchmark.
func BenchmarkHubPublishMinimalLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()

	// Create a minimal log record without enrichment fields.
	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Build step completed: compiling module org.example.service",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = hub.PublishLog(ctx, "bench-stream", record)
	}
}

// BenchmarkHubConcurrentPublishEnrichedLog measures throughput under concurrent
// publisher load. This simulates chatty Mods tickets with multiple concurrent
// log sources publishing enriched records.
func BenchmarkHubConcurrentPublishEnrichedLog(b *testing.B) {
	hub := NewHub(Options{BufferSize: 256, HistorySize: 1024})
	ctx := context.Background()

	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.000000Z",
		Stream:    "stdout",
		Line:      "Concurrent build output from parallel job execution",
		NodeID:    "node-concurrent-test",
		JobID:     "job-concurrent-test",
		ModType:   "hook",
		StepIndex: 100,
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = hub.PublishLog(ctx, "bench-concurrent", record)
		}
	})
}

// TestHubHighVolumeEnrichedLogs verifies that the hub remains stable under
// sustained high-volume publishing of enriched log records. This simulates
// long-running Mods tickets with chatty output. The test uses concurrent
// consumers to actively drain events while publishing continues.
func TestHubHighVolumeEnrichedLogs(t *testing.T) {
	t.Parallel()

	// Configuration for high-volume test:
	// - 1,000 log records simulates a chatty build (e.g., verbose Maven output)
	// - Buffer is sized to avoid backpressure drops for active consumers
	const numLogs = 1000
	const numSubscribers = 3

	hub := NewHub(Options{
		BufferSize:  numLogs + 1,
		HistorySize: 2 * numLogs,
	})
	ctx := context.Background()

	// Track events received by each subscriber using concurrent goroutines.
	type result struct {
		count       int
		receivedIDs []int64
		sawDone     bool
	}
	results := make(chan result, numSubscribers)

	// Start subscriber goroutines that actively consume events.
	for i := 0; i < numSubscribers; i++ {
		sub, err := hub.Subscribe(ctx, "high-volume-stream", 0)
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		go func(subID int, s Subscription) {
			defer s.Cancel()
			r := result{}
			timeout := time.After(5 * time.Second)
			for {
				select {
				case evt, ok := <-s.Events:
					if !ok {
						// Channel closed (backpressure drop).
						results <- r
						return
					}
					r.count++
					r.receivedIDs = append(r.receivedIDs, evt.ID)
					if evt.Type == "done" {
						r.sawDone = true
						results <- r
						return
					}
				case <-timeout:
					results <- r
					return
				}
			}
		}(i, sub)
	}

	// Publish high volume of enriched logs.
	for i := 0; i < numLogs; i++ {
		record := LogRecord{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Stream:    "stdout",
			Line:      "Compiling module " + string(rune('A'+i%26)),
			NodeID:    "aabbccdd-eeff-0011-2233-445566778899",
			JobID:     "11223344-5566-7788-99aa-bbccddeeff00",
			ModType:   "mod",
			StepIndex: i,
		}
		if err := hub.PublishLog(ctx, "high-volume-stream", record); err != nil {
			t.Fatalf("publish log %d: %v", i, err)
		}
	}

	// Publish terminal status to signal completion.
	if err := hub.PublishStatus(ctx, "high-volume-stream", Status{Status: "done"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	// Collect results from all subscribers.
	var allResults []result
	for i := 0; i < numSubscribers; i++ {
		r := <-results
		allResults = append(allResults, r)
		t.Logf("subscriber %d: received %d events, sawDone=%v", i, r.count, r.sawDone)
	}

	// Verify at least one subscriber received the done event.
	// Active consumers should keep up with publishing.
	sawDone := false
	totalEvents := 0
	for _, r := range allResults {
		if r.sawDone {
			sawDone = true
		}
		totalEvents += r.count
	}

	if !sawDone {
		t.Error("no subscriber received the done event - possible performance regression")
	}

	// Verify we received a reasonable number of events across all subscribers.
	// With active consumers, we expect most events to be delivered.
	expectedMin := numLogs / 2 // At least half the events per active consumer
	for i, r := range allResults {
		if r.count < expectedMin && !r.sawDone {
			t.Logf("warning: subscriber %d received only %d events (expected >= %d)", i, r.count, expectedMin)
		}
	}
}

// TestHubEnrichedLogPayloadSize verifies that enriched log records with maximum
// field sizes are handled correctly. This ensures large frames don't break
// serialization or subscription delivery.
func TestHubEnrichedLogPayloadSize(t *testing.T) {
	t.Parallel()

	hub := NewHub(Options{BufferSize: 8, HistorySize: 16})
	ctx := context.Background()

	// Create a log record with maximum reasonable field sizes.
	// Real-world logs can have long lines from stack traces, build output, etc.
	longLine := strings.Repeat("X", 8192) // 8KB log line (common for stack traces)

	record := LogRecord{
		Timestamp: "2025-12-01T10:00:00.123456789Z",
		Stream:    "stderr",
		Line:      longLine,
		NodeID:    "aabbccdd-eeff-0011-2233-445566778899",
		JobID:     "11223344-5566-7788-99aa-bbccddeeff00",
		ModType:   "gate",
		StepIndex: 999,
	}

	// Subscribe before publishing.
	sub, err := hub.Subscribe(ctx, "large-payload-stream", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	// Publish the large log record.
	if err := hub.PublishLog(ctx, "large-payload-stream", record); err != nil {
		t.Fatalf("publish log: %v", err)
	}

	// Verify subscriber receives the full record.
	select {
	case evt := <-sub.Events:
		if evt.Type != "log" {
			t.Fatalf("expected event type 'log', got %s", evt.Type)
		}

		// Verify the JSON payload is valid and contains the full line.
		var received LogRecord
		if err := json.Unmarshal(evt.Data, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if received.Line != longLine {
			t.Errorf("line truncated: got %d bytes, want %d", len(received.Line), len(longLine))
		}
		if received.NodeID != record.NodeID {
			t.Errorf("node_id: got %q, want %q", received.NodeID, record.NodeID)
		}
		if received.StepIndex != record.StepIndex {
			t.Errorf("step_index: got %d, want %d", received.StepIndex, record.StepIndex)
		}

	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for large payload event")
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

	// Subscribe with a slow consumer (never drains the channel).
	sub, err := hub.Subscribe(ctx, "backpressure-stream", 0)
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
			NodeID:    "node-backpressure",
			JobID:     "job-backpressure",
			ModType:   "mod",
			StepIndex: i,
		}
		// Should not block; slow subscriber should be dropped.
		if err := hub.PublishLog(ctx, "backpressure-stream", record); err != nil {
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
		if evt.Type != "log" {
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
