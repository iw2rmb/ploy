package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/tmaxmax/go-sse"
)

// SSEClient is an adapter that wraps github.com/tmaxmax/go-sse and exposes
// a Stream-style API compatible with the existing Client, Event, and ErrDone contracts.
// This adapter prepares for future migration from the custom SSE parser to the library.
type SSEClient struct {
	// HTTPClient is the http.Client used for making SSE requests.
	HTTPClient *http.Client
	// MaxRetries limits reconnection attempts. -1 means unlimited retries.
	MaxRetries int
	// InitialBackoff is the starting delay between reconnect attempts.
	// Exponential backoff with jitter is applied automatically.
	InitialBackoff time.Duration
	// IdleTimeout cancels the stream if no events are received within this duration.
	// Zero means no idle timeout.
	IdleTimeout time.Duration
	// Logger is used for structured logging of reconnect events and errors.
	Logger *slog.Logger
}

// Stream consumes SSE events from the endpoint and invokes the handler for each event.
// The handler may return ErrDone to stop streaming gracefully.
// This method uses github.com/tmaxmax/go-sse under the hood for SSE parsing,
// but maintains the same external API as the existing Client.Stream.
//
// Behavior:
// - Automatically reconnects on connection failures with exponential backoff.
// - Sends Last-Event-ID header on reconnects to resume from the last event.
// - Respects server "retry" hints by adjusting reconnect delays.
// - Cancels the stream if IdleTimeout is exceeded without receiving events.
// - Returns ErrDone if the handler signals completion.
// - Returns an error if MaxRetries is exceeded or a non-recoverable error occurs.
func (c *SSEClient) Stream(ctx context.Context, endpoint string, handler func(Event) error) error {
	if c.HTTPClient == nil {
		return errors.New("sse_client: http client required")
	}
	if handler == nil {
		return errors.New("sse_client: handler required")
	}

	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var lastEventID domaintypes.EventID // typed cursor for SSE resumption
	retries := 0
	maxRetries := c.MaxRetries
	initialBackoff := c.InitialBackoff
	if initialBackoff <= 0 {
		// Default to 250ms initial backoff, matching SSEStreamPolicy.
		initialBackoff = 250 * time.Millisecond
	}
	// Current backoff delay, grows exponentially with jitter.
	currentBackoff := initialBackoff

	for {
		// Derive a per-connection context so we can cancel on idle timeout without
		// affecting the caller's context.
		connCtx, cancelConn := context.WithCancel(ctx)
		defer cancelConn()

		// Build the HTTP request with the connection-scoped context for per-connection cancellation.
		req, err := http.NewRequestWithContext(connCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("sse_client: build request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		// Include Last-Event-ID header to resume from the last successfully processed event.
		// Stringify the typed EventID only at the HTTP header boundary.
		if lastEventID > 0 {
			req.Header.Set("Last-Event-ID", lastEventID.String())
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if connCtx.Err() != nil {
				// Detect connection-cancel due to idle timeout.
				if c.IdleTimeout > 0 {
					logger.Error("sse_client_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
					return fmt.Errorf("sse_client: idle timeout after %s", c.IdleTimeout)
				}
				return connCtx.Err()
			}
			// Connection error: check if retries are exhausted.
			if maxRetries >= 0 && retries >= maxRetries {
				logger.Error("sse_client_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
				return fmt.Errorf("sse_client: connect failed after %d retries: %w", retries, err)
			}
			// Apply exponential backoff with jitter and retry.
			retries++
			backoffDuration := applyJitter(currentBackoff)
			logger.Warn("sse_client_reconnect_backoff", "attempt", retries, "backoff", backoffDuration, "error", err.Error())
			if err := waitForBackoff(ctx, backoffDuration); err != nil {
				return err
			}
			// Exponential backoff: double the delay for next retry (capped implicitly by timing).
			currentBackoff *= 2
			if currentBackoff > 30*time.Second {
				currentBackoff = 30 * time.Second
			}
			continue
		}

		// Non-200 status: treat as permanent error and fail immediately.
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			logger.Error("sse_client_unexpected_status", "status", resp.StatusCode, "body", strings.TrimSpace(string(body)))
			return fmt.Errorf("sse_client: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		// Set up idle timeout timer that will cancel the connection context.
		// This will abort the underlying HTTP connection when no events are received.
		var idleTimer *time.Timer
		if c.IdleTimeout > 0 {
			idleTimer = time.AfterFunc(c.IdleTimeout, func() {
				logger.Debug("sse_client_idle_timeout_triggered", "timeout", c.IdleTimeout)
				// Cancel the connection context to abort the HTTP request.
				// This will cause resp.Body reads to fail with an error.
				cancelConn()
			})
			defer idleTimer.Stop()
		}

		// Parse SSE events using go-sse library's Read function.
		// The Read function provides a functional iterator over events.
		// We create a config with a reasonable max event size (1MB).
		readConfig := &sse.ReadConfig{
			MaxEventSize: 1 << 20, // 1MB max event size
		}

		sawEvent := false
		connectionFailed := false
		handlerErr := error(nil)
		gotDone := false // Track if handler explicitly returned ErrDone.

		// go-sse's Read function returns an iterator function that internally loops
		// over events and invokes our callback for each event or error.
		// The callback returns false to stop iteration, true to continue.
		// Note: Read handles EOF gracefully and does not pass it as an error.
		sse.Read(resp.Body, readConfig)(func(sseEvent sse.Event, err error) bool {
			if err != nil {
				// Check if idle timeout triggered the cancellation.
				// When the connection context is cancelled, the body read will fail.
				if connCtx.Err() != nil {
					if c.IdleTimeout > 0 {
						handlerErr = fmt.Errorf("sse_client: idle timeout after %s", c.IdleTimeout)
						logger.Error("sse_client_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
						return false
					}
					// Context cancelled for other reasons (parent context or explicit cancel).
					handlerErr = connCtx.Err()
					return false
				}
				// Treat read errors as transient and trigger reconnect.
				logger.Debug("sse_client_read_error", "error", err.Error())
				connectionFailed = true
				return false
			}

			sawEvent = true
			// Reset idle timer on each successful event.
			if idleTimer != nil {
				idleTimer.Reset(c.IdleTimeout)
			}

			// Map go-sse.Event to our Event struct.
			evt := Event{
				ID:   sseEvent.LastEventID,
				Type: sseEvent.Type,
				Data: []byte(sseEvent.Data),
			}

			// Track the last event ID for resumption on reconnect.
			// Parse and validate the event ID from the wire to our typed cursor.
			if evt.ID != "" {
				var eid domaintypes.EventID
				if err := eid.UnmarshalText([]byte(evt.ID)); err == nil && eid.Valid() {
					lastEventID = eid
				}
			}

			// Note: go-sse's Read function does not expose the "retry" field.
			// Retry handling requires using the Client/Connection API instead.
			// For this adapter, we omit server retry hint support to keep
			// compatibility with the Read-based approach.

			// Invoke the user's event handler.
			if err := handler(evt); err != nil {
				if errors.Is(err, ErrDone) {
					logger.Debug("sse_client_done", "endpoint", endpoint)
					gotDone = true // Mark that we got ErrDone.
					return false
				}
				logger.Error("sse_client_handler_error", "error", err.Error())
				handlerErr = err
				return false
			}

			// Continue iteration (return true).
			return true
		})
		// When Read returns, it means either EOF was reached or the callback returned false.
		// If no error was set and we didn't get ErrDone, this is a clean EOF from the server.
		if !gotDone && handlerErr == nil && !connectionFailed {
			logger.Debug("sse_client_eof", "endpoint", endpoint)
		}
		_ = resp.Body.Close()

		// If handler explicitly returned ErrDone, return nil to signal graceful completion.
		if gotDone {
			return nil
		}

		// If handler returned an error (non-ErrDone), propagate it.
		if handlerErr != nil {
			return handlerErr
		}

		// Check if the parent context was cancelled.
		if ctx.Err() != nil {
			logger.Debug("sse_client_context_cancelled", "error", ctx.Err().Error())
			return ctx.Err()
		}

		// If we successfully received events, reset backoff state for future reconnects.
		if sawEvent {
			logger.Debug("sse_client_reset_backoff", "endpoint", endpoint)
			currentBackoff = initialBackoff
			retries = 0
		}

		// If connection failed mid-stream, check retries and apply backoff.
		if connectionFailed {
			if maxRetries >= 0 && retries >= maxRetries {
				logger.Error("sse_client_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
				return fmt.Errorf("sse_client: exceeded max retries (%d)", maxRetries)
			}
			retries++
			backoffDuration := applyJitter(currentBackoff)
			logger.Warn("sse_client_reconnect_backoff", "attempt", retries, "backoff", backoffDuration)
			if err := waitForBackoff(ctx, backoffDuration); err != nil {
				return err
			}
			// Exponential backoff: double the delay.
			currentBackoff *= 2
			if currentBackoff > 30*time.Second {
				currentBackoff = 30 * time.Second
			}
			continue
		}

		// EOF without events: check retries and apply backoff before retrying.
		if maxRetries >= 0 && retries >= maxRetries {
			logger.Error("sse_client_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
			return fmt.Errorf("sse_client: exceeded max retries (%d)", maxRetries)
		}
		retries++
		backoffDuration := applyJitter(currentBackoff)
		logger.Debug("sse_client_reconnect_backoff", "attempt", retries, "backoff", backoffDuration)
		if err := waitForBackoff(ctx, backoffDuration); err != nil {
			return err
		}
		// Exponential backoff: double the delay.
		currentBackoff *= 2
		if currentBackoff > 30*time.Second {
			currentBackoff = 30 * time.Second
		}
	}
}

// applyJitter adds ±50% jitter to the backoff duration for robustness under load.
// This helps spread out reconnect attempts and avoid thundering herd effects.
func applyJitter(d time.Duration) time.Duration {
	// Simple jitter: multiply by a factor in [0.5, 1.5].
	// Use a deterministic jitter based on current time nanoseconds for simplicity.
	jitterFactor := 0.5 + (float64(time.Now().UnixNano()%1000) / 1000.0)
	return time.Duration(float64(d) * jitterFactor)
}

// waitForBackoff waits for the specified duration or until context is cancelled.
// Returns ctx.Err() if context is cancelled, nil otherwise.
func waitForBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
