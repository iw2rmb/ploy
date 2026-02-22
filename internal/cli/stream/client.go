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
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/tmaxmax/go-sse"
)

// ErrDone signals that the consumer has finished processing the stream.
var ErrDone = errors.New("stream: done")

// Client streams server-sent events with reconnection semantics.
type Client struct {
	HTTPClient  *http.Client
	MaxRetries  int           // -1 for unlimited retries
	IdleTimeout time.Duration // optional: cancel stream if no events for this duration
	Logger      *slog.Logger  // optional: logger for backoff and reconnect events
}

// Event represents a server-sent event frame.
type Event struct {
	ID    string
	Type  string
	Data  []byte
	Retry time.Duration
}

// reconnectState tracks retry/backoff state across reconnection attempts within a single Stream call.
type reconnectState struct {
	sb         *backoff.StatefulBackoff
	retries    int
	maxRetries int
	logger     *slog.Logger
}

// backoffOrFail checks whether retries are exhausted. If not, it increments the retry
// counter, applies exponential backoff, and waits. Returns a non-nil error to signal
// the caller should return (either retries exhausted or context cancelled).
// A nil return means the caller should continue (retry the connection).
func (rs *reconnectState) backoffOrFail(ctx context.Context, retriesMsg string, logAttrs ...any) error {
	if rs.maxRetries >= 0 && rs.retries >= rs.maxRetries {
		rs.logger.Error("stream_max_retries_exhausted", "retries", rs.retries)
		return fmt.Errorf("stream: %s", retriesMsg)
	}
	rs.retries++
	d := time.Duration(rs.sb.Apply())
	rs.logger.Log(ctx, slog.LevelDebug, "stream_reconnect_backoff", append([]any{"attempt", rs.retries, "backoff", d}, logAttrs...)...)
	return waitWithBackoff(ctx, d)
}

// Stream consumes events from the endpoint and invokes handler for each event.
// The handler may return ErrDone to stop streaming gracefully.
func (c Client) Stream(ctx context.Context, endpoint string, handler func(Event) error) error {
	if c.HTTPClient == nil {
		return errors.New("stream: http client required")
	}
	if handler == nil {
		return errors.New("stream: handler required")
	}

	policy := backoff.SSEStreamPolicy()
	if c.MaxRetries >= 0 {
		policy.MaxAttempts = c.MaxRetries
	}

	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}

	rs := &reconnectState{
		sb:         backoff.NewStatefulBackoff(policy),
		maxRetries: c.MaxRetries,
		logger:     logger,
	}

	var lastEventID domaintypes.EventID

	for {
		connCtx, cancelConn := context.WithCancel(ctx)
		defer cancelConn()

		req, err := http.NewRequestWithContext(connCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("stream: build request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		if lastEventID > 0 {
			req.Header.Set("Last-Event-ID", lastEventID.String())
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if connCtx.Err() != nil {
				if c.IdleTimeout > 0 {
					logger.Error("stream_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
					return fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
				}
				return connCtx.Err()
			}
			if err := rs.backoffOrFail(ctx, fmt.Sprintf("connect failed after %d retries: %v", rs.retries, err), "error", err.Error()); err != nil {
				return err
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			logger.Error("stream_unexpected_status", "status", resp.StatusCode, "body", strings.TrimSpace(string(body)))
			return fmt.Errorf("stream: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var idle *time.Timer
		if c.IdleTimeout > 0 {
			idle = time.AfterFunc(c.IdleTimeout, func() {
				logger.Debug("stream_idle_timeout_triggered", "timeout", c.IdleTimeout)
				cancelConn()
			})
		}
		var sawEvent bool
		var connectionFailed bool
		var handlerErr error
		var gotDone bool

		readConfig := &sse.ReadConfig{
			MaxEventSize: 1 << 20,
		}

		sse.Read(resp.Body, readConfig)(func(sseEvent sse.Event, err error) bool {
			if err != nil {
				if connCtx.Err() != nil {
					if c.IdleTimeout > 0 {
						handlerErr = fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
						logger.Error("stream_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
						return false
					}
					handlerErr = connCtx.Err()
					return false
				}
				// Treat read errors as transient and trigger a reconnect loop
				// instead of failing the entire stream immediately. This
				// improves resilience on flaky TLS/HTTP/2 links.
				logger.Debug("stream_read_error", "error", err.Error())
				connectionFailed = true
				return false
			}

			sawEvent = true
			if idle != nil {
				idle.Reset(c.IdleTimeout)
			}

			event := Event{
				ID:   sseEvent.LastEventID,
				Type: sseEvent.Type,
				Data: []byte(sseEvent.Data),
			}

			if event.ID != "" {
				var eid domaintypes.EventID
				if err := eid.UnmarshalText([]byte(event.ID)); err == nil && eid.Valid() {
					lastEventID = eid
				}
			}

			if err := handler(event); err != nil {
				if errors.Is(err, ErrDone) {
					logger.Debug("stream_done", "endpoint", endpoint)
					gotDone = true
					return false
				}
				logger.Error("stream_handler_error", "error", err.Error())
				handlerErr = err
				return false
			}

			return true
		})

		if !gotDone && handlerErr == nil && !connectionFailed {
			logger.Debug("stream_eof", "endpoint", endpoint)
		}
		_ = resp.Body.Close()
		if idle != nil {
			idle.Stop()
		}

		if gotDone {
			return nil
		}
		if handlerErr != nil {
			return handlerErr
		}
		if ctx.Err() != nil {
			logger.Debug("stream_context_cancelled", "error", ctx.Err().Error())
			return ctx.Err()
		}

		if sawEvent {
			logger.Debug("stream_reset_backoff", "endpoint", endpoint)
			rs.sb.Reset()
			rs.retries = 0
		}

		msg := fmt.Sprintf("exceeded max retries (%d)", rs.retries)
		if connectionFailed {
			if err := rs.backoffOrFail(ctx, msg); err != nil {
				return err
			}
			continue
		}

		// EOF without events
		if err := rs.backoffOrFail(ctx, msg); err != nil {
			return err
		}
	}
}

// waitWithBackoff waits for the specified duration or until context is cancelled.
func waitWithBackoff(ctx context.Context, d time.Duration) error {
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
