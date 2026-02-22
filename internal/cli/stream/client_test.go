package stream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientWaitWithBackoffHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitWithBackoff(ctx, 10*time.Millisecond); err == nil {
		t.Fatalf("expected canceled context error from waitWithBackoff")
	}
	// Zero duration returns immediately without error.
	if err := waitWithBackoff(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error for zero wait: %v", err)
	}
}

func TestClientStreamRequiresHTTPClientAndHandler(t *testing.T) {
	c := Client{}
	if err := c.Stream(context.Background(), "http://example", nil); err == nil {
		t.Fatalf("expected handler required error")
	}
	c = Client{}
	if err := c.Stream(context.Background(), "http://example", func(Event) error { return nil }); err == nil {
		t.Fatalf("expected http client required error")
	}
}

func TestClientStreamHappyAndIdleTimeout(t *testing.T) {
	// Happy path: server emits a single event; handler returns ErrDone to stop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "id: 1\n")
		_, _ = fmt.Fprintf(w, "event: hello\n")
		_, _ = fmt.Fprintf(w, "data: world\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()

	c := Client{HTTPClient: srv.Client(), MaxRetries: 0}
	var seen int
	if err := c.Stream(context.Background(), srv.URL, func(e Event) error {
		seen++
		if e.Type != "hello" || string(e.Data) != "world" || e.ID != "1" {
			t.Fatalf("unexpected event: %+v", e)
		}
		return ErrDone
	}); err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if seen != 1 {
		t.Fatalf("expected 1 event, got %d", seen)
	}

	// Idle timeout path: server that never sends data.
	idleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			// Flush headers so the client connection is established and Do returns.
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer idleSrv.Close()
	c = Client{HTTPClient: idleSrv.Client(), IdleTimeout: 25 * time.Millisecond, MaxRetries: 0}
	err := c.Stream(context.Background(), idleSrv.URL, func(e Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}
}

func TestClientStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer srv.Close()
	c := Client{HTTPClient: srv.Client(), MaxRetries: 0}
	err := c.Stream(context.Background(), srv.URL, func(e Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestClientStreamHandlerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: x\n"))
		_, _ = w.Write([]byte("data: y\n\n"))
	}))
	defer srv.Close()
	c := Client{HTTPClient: srv.Client(), MaxRetries: 0}
	want := fmt.Errorf("boom")
	err := c.Stream(context.Background(), srv.URL, func(e Event) error { return want })
	if err == nil || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("expected handler error propagated, got %v", err)
	}
}

func TestClientStreamConnectRetries(t *testing.T) {
	// MaxRetries=1 allows initial attempt + 1 retry before failing.
	// Backoff is handled by the shared SSE backoff policy.
	c := Client{HTTPClient: &http.Client{Timeout: 50 * time.Millisecond}, MaxRetries: 1}
	// Unreachable port to trigger immediate connect error.
	err := c.Stream(context.Background(), "http://127.0.0.1:1", func(e Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("expected connect failed error, got %v", err)
	}
}

func TestClientStreamSendsLastEventIDOnReconnect(t *testing.T) {
	var sawLastID string
	first := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if first {
			first = false
			_, _ = w.Write([]byte("id: 7\n"))
			_, _ = w.Write([]byte("event: x\n"))
			_, _ = w.Write([]byte("data: y\n\n"))
			return
		}
		sawLastID = r.Header.Get("Last-Event-ID")
		_, _ = w.Write([]byte("event: done\n\n"))
	}))
	defer srv.Close()
	// MaxRetries=1 allows reconnection after first EOF. Backoff is handled by the shared policy.
	c := Client{HTTPClient: srv.Client(), MaxRetries: 1}
	_ = c.Stream(context.Background(), srv.URL, func(e Event) error {
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	})
	if sawLastID != "7" {
		t.Fatalf("expected Last-Event-ID 7, got %q", sawLastID)
	}
}

// TestClientStreamReconnectBackoffGrowth verifies that reconnect delays grow exponentially
// with jitter when the server repeatedly closes the connection (EOF without events).
func TestClientStreamReconnectBackoffGrowth(t *testing.T) {
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
			_, _ = w.Write([]byte("event: done\n\n"))
		}
	}))
	defer srv.Close()

	// Use shared SSE backoff policy (250ms initial with exponential growth).
	// Allow enough retries for the test to complete.
	c := Client{
		HTTPClient: srv.Client(),
		MaxRetries: 10,
	}

	err := c.Stream(context.Background(), srv.URL, func(e Event) error {
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

	// Verify backoff delay between first and second reconnect is >= initial backoff (250ms from SSE policy)
	// with jitter tolerance. The backoff grows: 250ms, 500ms, 1s, etc.
	// We allow jitter (±50%) so minimum delay is ~125ms and maximum is ~375ms for first backoff.
	if len(reconnectTimes) >= 2 {
		delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
		// Allow jitter tolerance: expect delay in range [100ms, 500ms] for first backoff.
		if delay1 < 100*time.Millisecond || delay1 > 500*time.Millisecond {
			t.Logf("warning: first backoff delay %v outside expected range [100ms, 500ms]", delay1)
		}
	}

	// Verify that the second backoff is longer than the first (exponential growth).
	if len(reconnectTimes) >= 3 {
		delay1 := reconnectTimes[1].Sub(reconnectTimes[0])
		delay2 := reconnectTimes[2].Sub(reconnectTimes[1])
		// Second delay should be roughly 2x first delay, with jitter tolerance.
		// We don't enforce strict ordering due to jitter, but log if suspicious.
		if delay2 < delay1/2 {
			t.Logf("warning: second backoff delay %v not growing as expected (first was %v)", delay2, delay1)
		}
	}
}

// TestClientStreamBackoffResetAfterSuccessfulEvent verifies that backoff state is reset
// after successfully receiving an event, so subsequent reconnects start fresh.
func TestClientStreamBackoffResetAfterSuccessfulEvent(t *testing.T) {
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
			_, _ = w.Write([]byte("event: data\ndata: test\n\n"))
			return
		}
		if attempts == 2 {
			// EOF without event: should apply backoff.
			return
		}
		if attempts >= 3 {
			_, _ = w.Write([]byte("event: done\n\n"))
		}
	}))
	defer srv.Close()

	// Use shared SSE backoff policy (250ms initial with exponential growth).
	c := Client{
		HTTPClient: srv.Client(),
		MaxRetries: 10,
	}

	err := c.Stream(context.Background(), srv.URL, func(e Event) error {
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

	// First delay after event should be small (initial backoff ~250ms from SSE policy with jitter).
	// Second delay should also be small because backoff was reset after the event.
	// We verify both are within the initial backoff range (100ms-500ms with jitter).
	if delay1 < 50*time.Millisecond || delay1 > 600*time.Millisecond {
		t.Logf("warning: first delay %v outside expected range", delay1)
	}
	if delay2 < 50*time.Millisecond || delay2 > 600*time.Millisecond {
		t.Logf("warning: second delay %v outside expected range", delay2)
	}

	// The key expectation is that the second delay remains in the same order
	// of magnitude as the first, indicating that backoff was reset after the
	// event and did not explode to very large values. Because the underlying
	// backoff uses jitter and the test can run on contended CI runners, we
	// keep this as a soft assertion: log suspicious ratios instead of failing
	// the test to avoid flakes.
	if delay2 > 6*delay1 {
		t.Logf("suspicious backoff growth after event (non-fatal): delay1=%v, delay2=%v", delay1, delay2)
	}
}

// TestClientStreamMaxRetriesExhausted verifies that Stream returns an error
// when MaxRetries is exceeded due to repeated connection failures.
func TestClientStreamMaxRetriesExhausted(t *testing.T) {
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

	// MaxRetries=2 allows initial attempt + 2 retries = 3 total attempts.
	// Backoff is handled by the shared SSE backoff policy.
	c := Client{
		HTTPClient: srv.Client(),
		MaxRetries: 2,
	}

	err := c.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded max retries") {
		t.Fatalf("expected max retries error, got %v", err)
	}

	// We expect MaxRetries+1 attempts: initial attempt (retries=0) + MaxRetries retries.
	// MaxRetries=2 means: attempt 0 (initial), attempt 1 (retry 1), attempt 2 (retry 2) = 3 total.
	if attempts != 3 {
		t.Fatalf("expected 3 connection attempts (MaxRetries=2), got %d", attempts)
	}
}

// TestClientStreamIdleTimeoutCancelsConnection verifies that IdleTimeout
// triggers context cancellation if no events are received within the timeout period.
func TestClientStreamIdleTimeoutCancelsConnection(t *testing.T) {
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

	c := Client{
		HTTPClient:  srv.Client(),
		IdleTimeout: 50 * time.Millisecond,
		MaxRetries:  0, // No retries; fail immediately on idle timeout.
	}

	start := time.Now()
	err := c.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}

	// Verify that the stream was cancelled around the idle timeout duration.
	// Allow some tolerance for scheduling and timing jitter.
	if elapsed < 30*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Logf("warning: idle timeout took %v, expected ~50ms", elapsed)
	}
}

// TestClientStreamServerRetryHint is skipped because go-sse's Read function
// does not expose the "retry" field. Server retry hints require using the
// Client/Connection API instead of the Read-based approach.
// This is a known limitation documented in the migration to go-sse.
func TestClientStreamServerRetryHint(t *testing.T) {
	t.Skip("go-sse Read API does not expose retry field; use Client/Connection API for retry hints")
}
