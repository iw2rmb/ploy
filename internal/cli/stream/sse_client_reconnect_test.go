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
