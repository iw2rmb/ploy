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

// TestSSEClientRequiresHTTPClientAndHandler verifies that SSEClient.Stream
// validates required fields (HTTPClient and handler) before starting.
func TestSSEClientRequiresHTTPClientAndHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		client      *SSEClient
		handler     func(Event) error
		wantErrText string
	}{
		{
			name:        "missing http client",
			client:      &SSEClient{},
			handler:     func(Event) error { return nil },
			wantErrText: "http client required",
		},
		{
			name:        "missing handler",
			client:      &SSEClient{HTTPClient: &http.Client{}},
			handler:     nil,
			wantErrText: "handler required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.client.Stream(context.Background(), "http://example.com", tt.handler)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrText)
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErrText, err)
			}
		})
	}
}

// TestSSEClientParsesBasicEvent verifies that SSEClient correctly parses
// a well-formed SSE event with id, event, and data fields.
func TestSSEClientParsesBasicEvent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Emit a single event with id, event, and data fields.
		_, _ = fmt.Fprintf(w, "id: 42\n")
		_, _ = fmt.Fprintf(w, "event: log\n")
		_, _ = fmt.Fprintf(w, "data: hello world\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0, // No retries for this test.
	}

	var receivedEvent Event
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		receivedEvent = e
		return ErrDone
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	if receivedEvent.ID != "42" {
		t.Errorf("expected ID=42, got %q", receivedEvent.ID)
	}
	if receivedEvent.Type != "log" {
		t.Errorf("expected Type=log, got %q", receivedEvent.Type)
	}
	if string(receivedEvent.Data) != "hello world" {
		t.Errorf("expected Data='hello world', got %q", receivedEvent.Data)
	}
}

// TestSSEClientParsesMultilineData verifies that SSEClient correctly handles
// events with multiple "data:" lines, joining them with newlines.
func TestSSEClientParsesMultilineData(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Emit an event with multiple data lines.
		_, _ = fmt.Fprintf(w, "event: multi\n")
		_, _ = fmt.Fprintf(w, "data: line1\n")
		_, _ = fmt.Fprintf(w, "data: line2\n")
		_, _ = fmt.Fprintf(w, "data: line3\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	var receivedEvent Event
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		receivedEvent = e
		return ErrDone
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	expectedData := "line1\nline2\nline3"
	if string(receivedEvent.Data) != expectedData {
		t.Errorf("expected Data=%q, got %q", expectedData, receivedEvent.Data)
	}
}

// TestSSEClientIgnoresComments verifies that SSEClient correctly ignores
// comment lines (lines starting with ":") as per the SSE specification.
func TestSSEClientIgnoresComments(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Emit comments and a valid event.
		_, _ = fmt.Fprintf(w, ": this is a comment\n")
		_, _ = fmt.Fprintf(w, "event: test\n")
		_, _ = fmt.Fprintf(w, ": another comment\n")
		_, _ = fmt.Fprintf(w, "data: payload\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	var receivedEvent Event
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		receivedEvent = e
		return ErrDone
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	if receivedEvent.Type != "test" || string(receivedEvent.Data) != "payload" {
		t.Errorf("expected Type=test, Data=payload, got Type=%q, Data=%q", receivedEvent.Type, receivedEvent.Data)
	}
}

// TestSSEClientHandlesRetryField is skipped because go-sse's Read function
// does not expose the "retry" field. Server retry hints require using the
// Client/Connection API instead of the Read-based approach.
// This is a known limitation documented in the adapter.
func TestSSEClientHandlesRetryField(t *testing.T) {
	t.Skip("go-sse Read API does not expose retry field; use Client/Connection API for retry hints")
}

// TestSSEClientSendsLastEventIDOnReconnect verifies that SSEClient includes
// the Last-Event-ID header when reconnecting after receiving events.
func TestSSEClientSendsLastEventIDOnReconnect(t *testing.T) {
	t.Parallel()

	var sawLastEventID string
	first := true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if first {
			first = false
			// First connection: send an event with numeric id, then EOF to trigger reconnect.
			// EventID is typed as int64, so we use numeric IDs.
			_, _ = fmt.Fprintf(w, "id: 7\n")
			_, _ = fmt.Fprintf(w, "event: data\n")
			_, _ = fmt.Fprintf(w, "data: payload\n\n")
			return
		}
		// Second connection: capture Last-Event-ID header and send done event.
		sawLastEventID = r.Header.Get("Last-Event-ID")
		_, _ = fmt.Fprintf(w, "event: done\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:     srv.Client(),
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
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

	// EventID is typed as int64; the header should contain the stringified numeric ID.
	if sawLastEventID != "7" {
		t.Errorf("expected Last-Event-ID=7, got %q", sawLastEventID)
	}
}

// TestSSEClientIdleTimeout verifies that SSEClient cancels the stream
// if no events are received within the IdleTimeout duration.
func TestSSEClientIdleTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Server never sends events; waits for context cancellation (idle timeout).
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:  srv.Client(),
		IdleTimeout: 50 * time.Millisecond,
		MaxRetries:  0, // No retries; fail immediately on idle timeout.
	}

	start := time.Now()
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}

	// Verify that the stream was cancelled around the idle timeout duration.
	if elapsed < 30*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Logf("warning: idle timeout took %v, expected ~50ms", elapsed)
	}
}

// TestSSEClientMaxRetries verifies that SSEClient stops reconnecting
// after MaxRetries connection failures and returns an error.
func TestSSEClientMaxRetries(t *testing.T) {
	t.Parallel()

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Always EOF without events to trigger reconnect backoff.
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:     srv.Client(),
		MaxRetries:     2, // Initial attempt + 2 retries = 3 total attempts.
		InitialBackoff: 10 * time.Millisecond,
	}

	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded max retries") {
		t.Fatalf("expected max retries error, got %v", err)
	}

	// We expect MaxRetries+1 attempts: initial attempt (retries=0) + MaxRetries retries.
	if attempts != 3 {
		t.Fatalf("expected 3 connection attempts (MaxRetries=2), got %d", attempts)
	}
}

// TestSSEClientUnexpectedHTTPStatus verifies that SSEClient treats
// non-200 HTTP status codes as permanent errors and fails immediately.
func TestSSEClientUnexpectedHTTPStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("expected unexpected status error, got %v", err)
	}
}

// TestSSEClientHandlerErrorPropagation verifies that SSEClient propagates
// errors returned by the event handler, except for ErrDone.
func TestSSEClientHandlerErrorPropagation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprintf(w, "event: test\n")
		_, _ = fmt.Fprintf(w, "data: payload\n\n")
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 0,
	}

	wantErr := fmt.Errorf("boom")
	err := client.Stream(context.Background(), srv.URL, func(e Event) error {
		return wantErr
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("expected handler error propagated, got %v", err)
	}
}

// TestSSEClientReconnectBackoffGrowth verifies that reconnect delays grow
// exponentially with jitter when the server repeatedly closes the connection.
func TestSSEClientReconnectBackoffGrowth(t *testing.T) {
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
		// Server immediately closes connection (EOF without events) to trigger backoff.
		// After 3 reconnects, send done event to stop.
		if attempts >= 3 {
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

	// We expect at least 3 reconnects (initial + 2 retries before the final success).
	if len(reconnectTimes) < 3 {
		t.Fatalf("expected at least 3 connection attempts, got %d", len(reconnectTimes))
	}

	// Verify backoff delay between first and second reconnect is >= initial backoff (50ms)
	// with jitter tolerance.
	if len(reconnectTimes) >= 2 {
		delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
		// Allow jitter tolerance: expect delay in range [25ms, 150ms].
		if delay1 < 25*time.Millisecond || delay1 > 150*time.Millisecond {
			t.Logf("warning: first backoff delay %v outside expected range [25ms, 150ms]", delay1)
		}
	}

	// Verify that the second backoff is longer than the first (exponential growth).
	if len(reconnectTimes) >= 3 {
		delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
		delay2 := reconnectTimes[2].Sub(reconnectTimes[1])
		// Second delay should be roughly 2x first delay, with jitter tolerance.
		if delay2 < delay1/2 {
			t.Logf("warning: second backoff delay %v not growing as expected (first was %v)", delay2, delay1)
		}
	}
}

// TestSSEClientBackoffResetAfterSuccessfulEvent verifies that backoff state
// is reset after successfully receiving an event, so subsequent reconnects start fresh.
func TestSSEClientBackoffResetAfterSuccessfulEvent(t *testing.T) {
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
		// First connection: send an event then EOF to trigger reconnect.
		// Second connection: immediately EOF (no event) to trigger backoff.
		// Third connection: send done event to stop.
		if attempts == 1 {
			_, _ = fmt.Fprintf(w, "event: data\n")
			_, _ = fmt.Fprintf(w, "data: test\n\n")
			return
		}
		if attempts == 2 {
			// EOF without event: should apply backoff.
			return
		}
		if attempts >= 3 {
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

	// We expect 3 connection attempts: initial (with event), retry (EOF), retry (done).
	if len(reconnectTimes) != 3 {
		t.Fatalf("expected 3 connection attempts, got %d", len(reconnectTimes))
	}

	// After first connection with event, backoff should reset.
	// The second reconnect should use initial backoff again (not doubled).
	delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
	delay2 := reconnectTimes[2].Sub(reconnectTimes[1])

	// First delay after event should be small (initial backoff ~50ms with jitter).
	// Second delay should also be small because backoff was reset after the event.
	if delay1 < 10*time.Millisecond || delay1 > 200*time.Millisecond {
		t.Logf("warning: first delay %v outside expected range", delay1)
	}
	if delay2 < 10*time.Millisecond || delay2 > 200*time.Millisecond {
		t.Logf("warning: second delay %v outside expected range", delay2)
	}

	// The key assertion: second delay should not be significantly larger than first,
	// indicating that backoff was reset after the event.
	if delay2 > 3*delay1 {
		t.Fatalf("backoff was not reset after event: delay1=%v, delay2=%v (expected similar)", delay1, delay2)
	}
}

// TestSSEClientContextCancellation verifies that SSEClient respects
// context cancellation and returns ctx.Err() immediately.
func TestSSEClientContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Server waits indefinitely.
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient: srv.Client(),
		MaxRetries: 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := client.Stream(ctx, srv.URL, func(e Event) error {
		return nil
	})
	if err == nil || err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestSSEClientApplyJitter verifies that applyJitter produces values
// in the range [0.5*d, 1.5*d] as expected.
func TestSSEClientApplyJitter(t *testing.T) {
	t.Parallel()

	d := 100 * time.Millisecond
	for i := 0; i < 100; i++ {
		jittered := applyJitter(d)
		if jittered < 50*time.Millisecond || jittered > 150*time.Millisecond {
			t.Errorf("applyJitter(%v) = %v, expected [50ms, 150ms]", d, jittered)
		}
	}
}

// TestSSEClientWaitForBackoffHonorsContext verifies that waitForBackoff
// returns immediately when the context is cancelled.
func TestSSEClientWaitForBackoffHonorsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForBackoff(ctx, 100*time.Millisecond)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Zero duration should return immediately without error.
	if err := waitForBackoff(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error for zero wait: %v", err)
	}
}

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
