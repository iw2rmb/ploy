package stream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// ErrDone signals that the consumer has finished processing the stream.
var ErrDone = errors.New("stream: done")

// Client streams server-sent events with reconnection semantics.
type Client struct {
	HTTPClient   *http.Client
	MaxRetries   int           // -1 for unlimited retries
	RetryBackoff time.Duration // deprecated: wait between reconnect attempts (use backoff policy instead)
	IdleTimeout  time.Duration // optional: cancel stream if no events for this duration
	Logger       *slog.Logger  // optional: logger for backoff and reconnect events
}

// Event represents a server-sent event frame.
type Event struct {
	ID    string
	Type  string
	Data  []byte
	Retry time.Duration
}

// Stream consumes events from the endpoint and invokes handler for each event.
// The handler may return ErrDone to stop streaming gracefully.
// Uses shared backoff policy for reconnects; respects MaxRetries, IdleTimeout, and server retry hints.
func (c Client) Stream(ctx context.Context, endpoint string, handler func(Event) error) error {
	if c.HTTPClient == nil {
		return errors.New("stream: http client required")
	}
	if handler == nil {
		return errors.New("stream: handler required")
	}

	// Use shared backoff policy for exponential reconnect delays with jitter.
	policy := backoff.SSEStreamPolicy()
	// If client specifies MaxRetries, apply it to the policy; -1 means unlimited.
	if c.MaxRetries >= 0 {
		policy.MaxAttempts = c.MaxRetries
	}
	// If client specifies a legacy RetryBackoff, use it as InitialInterval; otherwise use policy default.
	if c.RetryBackoff > 0 {
		policy.InitialInterval = c.RetryBackoff
	}

	// Create a stateful backoff manager to track exponential backoff state across reconnects.
	// This allows backoff to grow on repeated failures and reset on successful events.
	sb := backoff.NewStatefulBackoff(policy)

	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var lastID string
	retries := 0
	maxRetries := c.MaxRetries

	for {
		// Derive a per-connection context so we can cancel on idle timeout without
		// affecting the caller's context.
		connCtx, cancelConn := context.WithCancel(ctx)
		defer cancelConn()

		req, err := http.NewRequestWithContext(connCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("stream: build request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		// Include Last-Event-ID header to resume from the last successfully processed event.
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if connCtx.Err() != nil {
				// Detect connection-cancel due to idle timeout.
				if c.IdleTimeout > 0 {
					logger.Error("stream_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
					return fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
				}
				return connCtx.Err()
			}
			// Connection error: check if retries are exhausted before backing off.
			if maxRetries >= 0 && retries >= maxRetries {
				logger.Error("stream_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
				return fmt.Errorf("stream: connect failed after %d retries: %w", retries, err)
			}
			// Apply backoff and retry.
			retries++
			backoffDuration := sb.Apply()
			logger.Warn("stream_reconnect_backoff", "attempt", retries, "backoff", backoffDuration, "error", err.Error())
			if err := c.waitWithBackoff(ctx, backoffDuration); err != nil {
				return err
			}
			continue
		}

		// Non-200 status: treat as permanent error and fail immediately.
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			logger.Error("stream_unexpected_status", "status", resp.StatusCode, "body", strings.TrimSpace(string(body)))
			return fmt.Errorf("stream: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		reader := bufio.NewReader(resp.Body)
		// Idle timer: cancels the connection if no events are received within IdleTimeout.
		var idle *time.Timer
		if c.IdleTimeout > 0 {
			idle = time.AfterFunc(c.IdleTimeout, func() {
				logger.Debug("stream_idle_timeout_triggered", "timeout", c.IdleTimeout)
				cancelConn()
			})
		}
		var sawEvent bool
		var connectionFailed bool
		for {
			event, err := readEvent(reader)
			if err != nil {
				if errors.Is(err, io.EOF) {
					// Clean EOF: server closed the stream gracefully.
					logger.Debug("stream_eof", "endpoint", endpoint)
					break
				}
				// Treat read errors as transient and trigger a reconnect loop
				// instead of failing the entire stream immediately. This
				// improves resilience on flaky TLS/HTTP/2 links.
				_ = resp.Body.Close()
				if connCtx.Err() != nil && c.IdleTimeout > 0 {
					logger.Error("stream_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
					return fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
				}
				logger.Debug("stream_read_error", "error", err.Error())
				connectionFailed = true
				break
			}
			sawEvent = true
			// Reset the idle timer on each successful event.
			if idle != nil {
				idle.Reset(c.IdleTimeout)
			}
			// Track Last-Event-ID for resumption on reconnect.
			if event.ID != "" {
				lastID = event.ID
			}
			// Server may send a "retry" field to override backoff delay.
			// Apply this to the stateful backoff by resetting it to the server-specified duration.
			if event.Retry > 0 {
				logger.Debug("stream_server_retry_hint", "retry", event.Retry)
				// To honor server retry hint, reset backoff to the specified initial interval.
				// This is a deviation from pure exponential backoff but aligns with SSE spec.
				policy.InitialInterval = event.Retry
				sb = backoff.NewStatefulBackoff(policy)
			}
			// Invoke the user's event handler.
			if err := handler(event); err != nil {
				_ = resp.Body.Close()
				if errors.Is(err, ErrDone) {
					logger.Debug("stream_done", "endpoint", endpoint)
					return nil
				}
				logger.Error("stream_handler_error", "error", err.Error())
				return err
			}
		}
		_ = resp.Body.Close()
		if idle != nil {
			idle.Stop()
		}

		// Check if the parent context was cancelled while processing events.
		if ctx.Err() != nil {
			logger.Debug("stream_context_cancelled", "error", ctx.Err().Error())
			return ctx.Err()
		}

		// If we successfully received events, reset the backoff state for future reconnects.
		if sawEvent {
			logger.Debug("stream_reset_backoff", "endpoint", endpoint)
			sb.Reset()
			retries = 0
		}

		// If connection failed mid-stream, check retries and apply backoff before reconnecting.
		if connectionFailed {
			if maxRetries >= 0 && retries >= maxRetries {
				logger.Error("stream_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
				return fmt.Errorf("stream: exceeded max retries (%d)", maxRetries)
			}
			retries++
			backoffDuration := sb.Apply()
			logger.Warn("stream_reconnect_backoff", "attempt", retries, "backoff", backoffDuration)
			if err := c.waitWithBackoff(ctx, backoffDuration); err != nil {
				return err
			}
			continue
		}

		// EOF without events: check retries, apply backoff, and retry.
		if maxRetries >= 0 && retries >= maxRetries {
			logger.Error("stream_max_retries_exhausted", "retries", retries, "endpoint", endpoint)
			return fmt.Errorf("stream: exceeded max retries (%d)", maxRetries)
		}
		retries++
		backoffDuration := sb.Apply()
		logger.Debug("stream_reconnect_backoff", "attempt", retries, "backoff", backoffDuration)
		if err := c.waitWithBackoff(ctx, backoffDuration); err != nil {
			return err
		}
	}
}

// waitWithBackoff waits for the specified duration or until context is cancelled.
// Used by the Stream method to apply backoff delays between reconnect attempts.
func (c Client) waitWithBackoff(ctx context.Context, d time.Duration) error {
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

func readEvent(r *bufio.Reader) (Event, error) {
	var evt Event
	var hasData bool

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if evt.Type != "" || hasData || evt.ID != "" || evt.Retry > 0 {
					return evt, nil
				}
			}
			return evt, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if evt.Type == "" && !hasData && evt.ID == "" && evt.Retry == 0 {
				continue
			}
			return evt, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field := line
		value := ""
		if idx := strings.Index(line, ":"); idx >= 0 {
			field = line[:idx]
			value = strings.TrimSpace(line[idx+1:])
		}
		switch field {
		case "event":
			evt.Type = value
		case "data":
			if hasData {
				evt.Data = append(evt.Data, '\n')
			}
			evt.Data = append(evt.Data, value...)
			hasData = true
		case "id":
			evt.ID = value
		case "retry":
			if ms, err := strconv.Atoi(value); err == nil && ms >= 0 {
				evt.Retry = time.Duration(ms) * time.Millisecond
			}
		}
	}
}
