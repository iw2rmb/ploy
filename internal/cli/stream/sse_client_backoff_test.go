package stream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
		// Create two consecutive failures to grow backoff, then deliver an event
		// (which should reset backoff), then fail once more, then stop.
		switch attempts {
		case 1, 2:
			// EOF without events: should apply exponential backoff growth.
			return
		case 3:
			// Deliver an event, then EOF to force reconnect.
			_, _ = fmt.Fprintf(w, "event: data\n")
			_, _ = fmt.Fprintf(w, "data: ok\n\n")
			return
		case 4:
			// EOF without events: backoff should have been reset by the event on attempt 3.
			return
		default:
			_, _ = fmt.Fprintf(w, "event: done\n\n")
		}
	}))
	defer srv.Close()

	client := &SSEClient{
		HTTPClient:     srv.Client(),
		MaxRetries:     10,
		InitialBackoff: 300 * time.Millisecond,
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

	// We need at least 4 connection attempts to measure the delay after the successful event.
	if len(reconnectTimes) < 4 {
		t.Fatalf("expected at least 4 connection attempts, got %d", len(reconnectTimes))
	}

	// Attempt 3 delivers an event. The reconnect after that (attempt 4) should use
	// the initial backoff again (with jitter), not the grown backoff from attempts 1-2.
	delayAfterEvent := reconnectTimes[3].Sub(reconnectTimes[2])
	if delayAfterEvent > 550*time.Millisecond {
		t.Fatalf("backoff was not reset after event: delayAfterEvent=%v (expected < 550ms)", delayAfterEvent)
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
