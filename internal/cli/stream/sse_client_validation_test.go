package stream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
