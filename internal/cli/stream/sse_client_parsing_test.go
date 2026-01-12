package stream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
