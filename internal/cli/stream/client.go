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
func (rs *reconnectState) backoffOrFail(ctx context.Context, retriesMsg string) error {
	if rs.maxRetries >= 0 && rs.retries >= rs.maxRetries {
		rs.logger.Error("stream_max_retries_exhausted", "retries", rs.retries)
		return fmt.Errorf("stream: %s", retriesMsg)
	}
	rs.retries++
	d := time.Duration(rs.sb.Apply())
	rs.logger.Log(ctx, slog.LevelDebug, "stream_reconnect_backoff", "attempt", rs.retries, "backoff", d)
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
		attempt := c.streamConnection(ctx, endpoint, lastEventID, handler, logger)
		lastEventID = attempt.lastEventID
		if !attempt.reconnect {
			return attempt.err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt.sawEvent {
			logger.Debug("stream_reset_backoff", "endpoint", endpoint)
			rs.sb.Reset()
			rs.retries = 0
		}

		msg := fmt.Sprintf("exceeded max retries (%d)", rs.retries)
		if attempt.err != nil {
			msg = fmt.Sprintf("%v after %d retries", attempt.err, rs.retries)
		}
		if err := rs.backoffOrFail(ctx, msg); err != nil {
			return err
		}
	}
}

type connectionResult struct {
	reconnect   bool
	err         error
	sawEvent    bool
	lastEventID domaintypes.EventID
}

func (c Client) streamConnection(ctx context.Context, endpoint string, lastEventID domaintypes.EventID, handler func(Event) error, logger *slog.Logger) connectionResult {
	result := connectionResult{lastEventID: lastEventID}
	connCtx, cancelConn := context.WithCancelCause(ctx)
	// A stream can reconnect indefinitely. Release this connection's context before
	// backoff so completed attempts do not remain attached to the caller.
	defer cancelConn(nil)

	req, err := http.NewRequestWithContext(connCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.err = fmt.Errorf("stream: build request: %w", err)
		return result
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", lastEventID.String())
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.err = ctx.Err()
		} else {
			result.reconnect = true
			result.err = fmt.Errorf("connect failed: %w", err)
		}
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		logger.Error("stream_unexpected_status", "status", resp.StatusCode, "body", strings.TrimSpace(string(body)))
		result.err = fmt.Errorf("stream: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return result
	}

	var idle *time.Timer
	if c.IdleTimeout > 0 {
		idle = time.AfterFunc(c.IdleTimeout, func() {
			logger.Debug("stream_idle_timeout_triggered", "timeout", c.IdleTimeout)
			cancelConn(fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout))
		})
		defer idle.Stop()
	}

	result.reconnect = true
	sse.Read(resp.Body, &sse.ReadConfig{MaxEventSize: 1 << 20})(func(sseEvent sse.Event, err error) bool {
		if err != nil {
			// Read errors can come from transient TLS/HTTP/2 failures.
			logger.Debug("stream_read_error", "error", err.Error())
			result.err = err
			return false
		}

		result.sawEvent = true
		if idle != nil {
			idle.Reset(c.IdleTimeout)
		}
		event := Event{
			ID:   sseEvent.LastEventID,
			Type: sseEvent.Type,
			Data: []byte(sseEvent.Data),
		}
		if event.ID != "" {
			var eventID domaintypes.EventID
			if err := eventID.UnmarshalText([]byte(event.ID)); err == nil && eventID.Valid() {
				result.lastEventID = eventID
			}
		}

		if err := handler(event); err != nil {
			result.reconnect = false
			if errors.Is(err, ErrDone) {
				logger.Debug("stream_done", "endpoint", endpoint)
			} else {
				logger.Error("stream_handler_error", "error", err.Error())
				result.err = err
			}
			return false
		}
		return true
	})

	if result.reconnect {
		if ctx.Err() != nil {
			result.reconnect = false
			result.err = ctx.Err()
		} else if connCtx.Err() != nil {
			result.reconnect = false
			result.err = context.Cause(connCtx)
			logger.Error("stream_idle_timeout", "timeout", c.IdleTimeout, "endpoint", endpoint)
		} else if result.err == nil {
			logger.Debug("stream_eof", "endpoint", endpoint)
		}
	}
	return result
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
