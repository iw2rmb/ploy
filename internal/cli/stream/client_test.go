package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
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

func TestClientStreamDeliversEventUntilDone(t *testing.T) {
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
			_, _ = w.Write([]byte("id: invalid\ndata: z\n\n"))
			return
		}
		sawLastID = r.Header.Get("Last-Event-ID")
		_, _ = w.Write([]byte("event: done\n\n"))
	}))
	defer srv.Close()
	// MaxRetries=1 allows reconnection after first EOF. Backoff is handled by the shared policy.
	c := Client{HTTPClient: srv.Client(), MaxRetries: 1}
	if err := c.Stream(context.Background(), srv.URL, func(e Event) error {
		if e.Type == "done" {
			return ErrDone
		}
		return nil
	}); err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if sawLastID != "7" {
		t.Fatalf("expected Last-Event-ID 7, got %q", sawLastID)
	}
}

// Empty connections increase the delay; receiving an event resets both delay and retry budget.
func TestClientStreamBackoffGrowsAndResetsAfterEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initial := time.Duration(backoff.SSEStreamPolicy().InitialInterval)
		wantDelays := []time.Duration{initial, 2 * initial, 4 * initial, initial}
		attempts := 0
		var previousAttempt time.Time
		client := &http.Client{Transport: streamTransport(func(*http.Request) (*http.Response, error) {
			if attempts > 0 {
				delay := time.Since(previousAttempt)
				want := wantDelays[attempts-1]
				// The virtual clock excludes scheduler delays; only the policy's 50% jitter remains.
				if delay < want/2 || delay > want*3/2 {
					t.Fatalf("reconnect %d delay = %s, want %s with 50%% jitter", attempts, delay, want)
				}
			}
			previousAttempt = time.Now()
			attempts++
			data := ""
			switch attempts {
			case 4:
				data = "event: data\ndata: test\n\n"
			case 5:
				data = "event: done\n\n"
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(data))}, nil
		})}
		c := Client{HTTPClient: client, MaxRetries: 3}
		if err := c.Stream(context.Background(), "http://stream.test", func(event Event) error {
			if event.Type == "done" {
				return ErrDone
			}
			return nil
		}); err != nil {
			t.Fatalf("Stream error: %v", err)
		}
		if attempts != 5 {
			t.Fatalf("connection attempts = %d, want 5", attempts)
		}
	})
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

	err := c.Stream(context.Background(), srv.URL, func(e Event) error {
		return nil
	})

	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}
}

// TestClientStreamServerRetryHint is skipped because go-sse's Read function
// does not expose the "retry" field. Server retry hints require using the
// Client/Connection API instead of the Read-based approach.
// This is a known limitation documented in the migration to go-sse.
func TestClientStreamServerRetryHint(t *testing.T) {
	t.Skip("go-sse Read API does not expose retry field; use Client/Connection API for retry hints")
}

type streamTransport func(*http.Request) (*http.Response, error)

func (transport streamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

type recordedStreamBody struct {
	io.Reader
	closed chan struct{}
}

func (body *recordedStreamBody) Close() error {
	close(body.closed)
	return nil
}

type streamErrorReader struct{}

func (streamErrorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// Both connection resources are released before another request can start.
func TestClientStreamReleasesConnectionBeforeReconnect(t *testing.T) {
	for _, failure := range []string{"eof", "read", "connect"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			var previousContext context.Context
			var previousBody *recordedStreamBody
			assertReleased := func() {
				t.Helper()
				if previousContext.Err() != context.Canceled {
					t.Fatal("previous connection context remains active")
				}
				if previousBody != nil {
					select {
					case <-previousBody.closed:
					default:
						t.Fatal("previous response body remains open")
					}
				}
			}
			attempts := 0
			client := &http.Client{Transport: streamTransport(func(req *http.Request) (*http.Response, error) {
				if attempts > 0 {
					assertReleased()
				}
				attempts++
				previousContext = req.Context()
				if attempts == 1 && failure == "connect" {
					return nil, errors.New("connection failed")
				}
				var reader io.Reader = strings.NewReader("event: done\n\n")
				if attempts == 1 {
					reader = strings.NewReader("")
					if failure == "read" {
						reader = streamErrorReader{}
					}
				}
				previousBody = &recordedStreamBody{Reader: reader, closed: make(chan struct{})}
				return &http.Response{StatusCode: http.StatusOK, Body: previousBody}, nil
			})}
			c := Client{HTTPClient: client, MaxRetries: 1, IdleTimeout: time.Hour}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := c.Stream(ctx, "http://stream.test", func(Event) error {
				return fmt.Errorf("finished: %w", ErrDone)
			}); err != nil {
				t.Fatalf("Stream error: %v", err)
			}
			assertReleased()
			if attempts != 2 || ctx.Err() != nil {
				t.Fatalf("attempts = %d, caller error = %v", attempts, ctx.Err())
			}
		})
	}
}

// Caller cancellation retains its identity when the idle timer is enabled.
func TestClientStreamCallerCancellationIsNotIdleTimeout(t *testing.T) {
	for _, phase := range []string{"connect", "read"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var body *recordedStreamBody
			client := &http.Client{Transport: streamTransport(func(req *http.Request) (*http.Response, error) {
				if phase == "connect" {
					cancel()
					return nil, req.Context().Err()
				}
				body = &recordedStreamBody{
					Reader: cancelStreamReader{cancel: cancel, ctx: req.Context()},
					closed: make(chan struct{}),
				}
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})}
			c := Client{HTTPClient: client, MaxRetries: -1, IdleTimeout: time.Hour}
			err := c.Stream(ctx, "http://stream.test", func(Event) error { return nil })
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Stream error = %v, want context.Canceled", err)
			}
			if body != nil {
				select {
				case <-body.closed:
				default:
					t.Fatal("cancelled stream body remains open")
				}
			}
		})
	}
}

type cancelStreamReader struct {
	cancel context.CancelFunc
	ctx    context.Context
}

func (reader cancelStreamReader) Read([]byte) (int, error) {
	reader.cancel()
	<-reader.ctx.Done()
	return 0, reader.ctx.Err()
}
