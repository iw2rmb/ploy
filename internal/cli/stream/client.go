package stream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrDone signals that the consumer has finished processing the stream.
var ErrDone = errors.New("stream: done")

// Client streams server-sent events with reconnection semantics.
type Client struct {
	HTTPClient   *http.Client
	MaxRetries   int           // -1 for unlimited retries
	RetryBackoff time.Duration // wait between reconnect attempts
	IdleTimeout  time.Duration // optional: cancel stream if no events for this duration
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
func (c Client) Stream(ctx context.Context, endpoint string, handler func(Event) error) error {
	if c.HTTPClient == nil {
		return errors.New("stream: http client required")
	}
	if handler == nil {
		return errors.New("stream: handler required")
	}
	backoff := c.RetryBackoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}
	maxRetries := c.MaxRetries
	var lastID string
	retries := 0

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
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if connCtx.Err() != nil {
				// Detect connection-cancel due to idle timeout.
				if c.IdleTimeout > 0 {
					return fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
				}
				return connCtx.Err()
			}
			if maxRetries >= 0 && retries >= maxRetries {
				return fmt.Errorf("stream: connect failed after %d retries: %w", retries, err)
			}
			retries++
			if err := c.wait(ctx, backoff); err != nil {
				return err
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			return fmt.Errorf("stream: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		reader := bufio.NewReader(resp.Body)
		// Idle timer: cancels the connection if no events are received within IdleTimeout.
		var idle *time.Timer
		if c.IdleTimeout > 0 {
			idle = time.AfterFunc(c.IdleTimeout, func() { cancelConn() })
		}
		var sawEvent bool
		for {
			event, err := readEvent(reader)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				_ = resp.Body.Close()
				if connCtx.Err() != nil && c.IdleTimeout > 0 {
					return fmt.Errorf("stream: idle timeout after %s", c.IdleTimeout)
				}
				return fmt.Errorf("stream: read event: %w", err)
			}
			sawEvent = true
			if idle != nil {
				idle.Reset(c.IdleTimeout)
			}
			if event.ID != "" {
				lastID = event.ID
			}
			if event.Retry > 0 {
				backoff = event.Retry
			}
			if err := handler(event); err != nil {
				_ = resp.Body.Close()
				if errors.Is(err, ErrDone) {
					return nil
				}
				return err
			}
		}
		_ = resp.Body.Close()
		if idle != nil {
			idle.Stop()
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sawEvent {
			retries = 0
		}
		if maxRetries >= 0 && retries >= maxRetries {
			return fmt.Errorf("stream: exceeded max retries (%d)", maxRetries)
		}
		retries++
		if err := c.wait(ctx, backoff); err != nil {
			return err
		}
	}
}

func (c Client) wait(ctx context.Context, d time.Duration) error {
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
