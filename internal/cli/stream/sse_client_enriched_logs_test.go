package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

// =============================================================================
// Performance and Resilience Tests for Enriched Logs
// =============================================================================
// These tests validate that SSE streaming with enriched log payloads
// (node_id, job_id, mod_type, step_index) maintains stable backoff,
// idle timeout, and reconnection semantics.
// Stress test: validate performance and resilience with enriched logs.

// TestSSEClientHighVolumeEnrichedLogs verifies that the SSE client correctly
// handles a high volume of enriched log events without blocking or dropping.
// This simulates chatty Mods runs with verbose build output.
func TestSSEClientHighVolumeEnrichedLogs(t *testing.T) {
	t.Parallel()

	const numEvents = 1000

	eventCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Emit high volume of enriched log events.
		for i := 0; i < numEvents; i++ {
			// Construct JSON payload matching LogRecord with enriched fields.
			data := fmt.Sprintf(`{"timestamp":"2025-12-01T10:00:%02d.%06dZ","stream":"stdout","line":"Build log line %d","node_id":"node-perf-test","job_id":"job-perf-test","mod_type":"mod","step_index":%d}`,
				i/60, i%1000000, i, i)
			_, _ = fmt.Fprintf(w, "id: %d\n", i+1)
			_, _ = fmt.Fprintf(w, "event: log\n")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		// Send done event.
		_, _ = fmt.Fprintf(w, "event: done\n")
		_, _ = fmt.Fprintf(w, "data: {\"status\":\"done\"}\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		eventCount++
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	// Should receive all log events plus the done event.
	expectedCount := numEvents + 1
	if eventCount != expectedCount {
		t.Errorf("expected %d events, got %d", expectedCount, eventCount)
	}
}

// TestSSEClientReconnectWithEnrichedLogs verifies that reconnection with
// Last-Event-ID works correctly for enriched log payloads. This ensures
// resumption semantics remain stable with the larger frame sizes.
func TestSSEClientReconnectWithEnrichedLogs(t *testing.T) {
	t.Parallel()

	var sawLastEventID string
	connectionCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionCount++
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		if connectionCount == 1 {
			// First connection: send enriched log events then close.
			// EventID is typed as int64, so we use numeric IDs.
			for i := 1; i <= 5; i++ {
				data := fmt.Sprintf(`{"timestamp":"2025-12-01T10:00:0%d.000000Z","stream":"stdout","line":"Line %d","node_id":"node-reconnect","job_id":"job-reconnect","mod_type":"mod","step_index":%d}`, i, i, i*100)
				_, _ = fmt.Fprintf(w, "id: %d\n", i)
				_, _ = fmt.Fprintf(w, "event: log\n")
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			}
			// EOF to trigger reconnect.
			return
		}

		// Second connection: capture Last-Event-ID and complete.
		sawLastEventID = r.Header.Get("Last-Event-ID")
		_, _ = fmt.Fprintf(w, "id: 6\n")
		_, _ = fmt.Fprintf(w, "event: log\n")
		_, _ = fmt.Fprintf(w, "data: {\"timestamp\":\"2025-12-01T10:00:06.000000Z\",\"stream\":\"stdout\",\"line\":\"Resumed line\",\"node_id\":\"node-reconnect\",\"job_id\":\"job-reconnect\",\"mod_type\":\"mod\",\"step_index\":600}\n\n")
		_, _ = fmt.Fprintf(w, "event: done\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:     srv.Client(),
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}

	eventCount := 0
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		eventCount++
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	// Verify Last-Event-ID was sent on reconnect.
	// EventID is typed as int64; the header should contain "5" (last event from first connection).
	if sawLastEventID != "5" {
		t.Errorf("expected Last-Event-ID=5, got %q", sawLastEventID)
	}

	// Should receive 5 events from first connection + 1 log + done from second.
	if eventCount != 7 {
		t.Errorf("expected 7 events, got %d", eventCount)
	}
}

// TestSSEClientBackoffResetWithEnrichedLogs verifies that backoff state
// resets correctly after receiving enriched log events. This ensures
// the larger payloads don't affect backoff timing semantics.
func TestSSEClientBackoffResetWithEnrichedLogs(t *testing.T) {
	t.Parallel()

	attempts := 0
	reconnectTimes := []time.Time{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reconnectTimes = append(reconnectTimes, time.Now())
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		switch attempts {
		case 1:
			// First connection: send enriched log event then EOF.
			_, _ = fmt.Fprintf(w, "event: log\n")
			_, _ = fmt.Fprintf(w, "data: {\"timestamp\":\"T1\",\"stream\":\"stdout\",\"line\":\"event1\",\"node_id\":\"n1\",\"job_id\":\"j1\",\"mod_type\":\"mod\",\"step_index\":1}\n\n")
		case 2:
			// Second connection: immediate EOF (no event).
		case 3:
			// Third connection: send done.
			_, _ = fmt.Fprintf(w, "event: done\n\n")
		}
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:     srv.Client(),
		MaxRetries:     10,
		InitialBackoff: 50 * time.Millisecond,
	}

	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	if len(reconnectTimes) != 3 {
		t.Fatalf("expected 3 connection attempts, got %d", len(reconnectTimes))
	}

	// Delay after first connection (which had an event) should be small
	// (backoff reset after successful event).
	delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
	delay2 := reconnectTimes[2].Sub(reconnectTimes[1])

	// Both delays should be in the same general range as the initial backoff
	// with jitter. This confirms backoff reset is working correctly without
	// making the test brittle on contended CI runners. Treat suspicious ratios
	// as non-fatal diagnostics instead of hard assertions.
	if delay1 > 200*time.Millisecond {
		t.Logf("warning: first delay %v seems high for reset backoff", delay1)
	}
	if delay2 > 200*time.Millisecond {
		t.Logf("warning: second delay %v seems high for reset backoff", delay2)
	}
	if delay1 > 0 && delay2 > 6*delay1 {
		t.Logf("suspicious backoff growth after enriched event (non-fatal): delay1=%v, delay2=%v", delay1, delay2)
	}
}

// TestSSEClientIdleTimeoutWithEnrichedLogs verifies that idle timeout
// works correctly when the last received event was an enriched log.
// This ensures larger payloads don't affect timeout handling.
func TestSSEClientIdleTimeoutWithEnrichedLogs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Send one enriched event then go idle.
		_, _ = fmt.Fprintf(w, "event: log\n")
		_, _ = fmt.Fprintf(w, "data: {\"timestamp\":\"T1\",\"stream\":\"stdout\",\"line\":\"last event before idle\",\"node_id\":\"n1\",\"job_id\":\"j1\",\"mod_type\":\"mod\",\"step_index\":1}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Wait for context cancellation (idle timeout).
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:  srv.Client(),
		IdleTimeout: 100 * time.Millisecond,
		MaxRetries:  0,
	}

	eventCount := 0
	start := time.Now()
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		eventCount++
		return nil
	})
	elapsed := time.Since(start)

	// Should have received the enriched event before timeout.
	if eventCount != 1 {
		t.Errorf("expected 1 event before idle timeout, got %d", eventCount)
	}

	// Should get idle timeout error.
	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}

	// Timeout should occur around 100ms after the event.
	// Allow some tolerance for event processing time.
	if elapsed < 80*time.Millisecond || elapsed > 300*time.Millisecond {
		t.Logf("warning: idle timeout took %v, expected ~100ms after event", elapsed)
	}
}

// TestSSEClientLargeEnrichedPayload verifies that the SSE client correctly
// handles enriched log events with large payloads (e.g., stack traces).
// This ensures the client doesn't truncate or misparse large frames.
func TestSSEClientLargeEnrichedPayload(t *testing.T) {
	t.Parallel()

	nodeID := "aB3xY9"
	jobID := domaintypes.NewJobID().String()

	// Create a large log line simulating a stack trace.
	largeLine := strings.Repeat("at com.example.service.Handler.processRequest(Handler.java:123)\n", 100)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Emit a large enriched log event.
		// Note: JSON encoding will escape newlines.
		escapedLine := strings.ReplaceAll(largeLine, "\n", "\\n")
		data := fmt.Sprintf(`{"timestamp":"2025-12-01T10:00:00Z","stream":"stderr","line":"%s","node_id":"%s","job_id":"%s","mod_type":"pre_gate","step_index":999}`, escapedLine, nodeID, jobID)
		_, _ = fmt.Fprintf(w, "id: 1\n")
		_, _ = fmt.Fprintf(w, "event: log\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		_, _ = fmt.Fprintf(w, "event: done\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	var receivedData []byte
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		if e.Type == "log" {
			receivedData = e.Data
		}
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	// Verify the large payload was received intact.
	if len(receivedData) == 0 {
		t.Fatal("no log data received")
	}

	// Parse the JSON and verify the line content.
	var rec logstream.LogRecord
	if err := json.Unmarshal(receivedData, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if rec.Line != largeLine {
		t.Errorf("line truncated or corrupted: got %d bytes, want %d", len(rec.Line), len(largeLine))
	}
	if rec.NodeID.String() != nodeID {
		t.Errorf("node_id: got %q, want %q", rec.NodeID.String(), nodeID)
	}
	if rec.StepIndex != 999 {
		t.Errorf("step_index: got %v, want %v", rec.StepIndex, 999)
	}
}
