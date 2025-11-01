package stream

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadEventParsesFieldsAndMultiData(t *testing.T) {
	src := strings.Join([]string{
		": comment line",
		"id: 42",
		"event: log",
		"retry: 123",
		"data: hello",
		"data: world",
		"",
	}, "\n")
	evt, err := readEvent(bufio.NewReader(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("readEvent error: %v", err)
	}
	if evt.ID != "42" || evt.Type != "log" {
		t.Fatalf("unexpected id/type: %+v", evt)
	}
	if string(evt.Data) != "hello\nworld" {
		t.Fatalf("unexpected data: %q", evt.Data)
	}
	if evt.Retry != 123*time.Millisecond {
		t.Fatalf("unexpected retry: %v", evt.Retry)
	}
}

func TestReadEventEOFWithPartialFrame(t *testing.T) {
	// EOF after emitting a partial event should still return the event.
	src := "event: x\n"
	evt, err := readEvent(bufio.NewReader(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("readEvent error: %v", err)
	}
	if evt.Type != "x" {
		t.Fatalf("unexpected type: %v", evt)
	}
}

func TestClientWaitHonorsContext(t *testing.T) {
	c := Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.wait(ctx, 10*time.Millisecond); err == nil {
		t.Fatalf("expected canceled context error from wait")
	}
	// Zero duration returns immediately without error.
	if err := c.wait(context.Background(), 0); err != nil {
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
	c := Client{HTTPClient: &http.Client{Timeout: 50 * time.Millisecond}, MaxRetries: 1, RetryBackoff: 10 * time.Millisecond}
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
	c := Client{HTTPClient: srv.Client(), MaxRetries: 1, RetryBackoff: 10 * time.Millisecond}
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
