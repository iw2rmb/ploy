package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/cmd/ploy/stream"
)

// Format controls job log rendering.
type Format string

const (
	// FormatStructured prints timestamp, stream, and line.
	FormatStructured Format = "structured"
	// FormatRaw prints only the log line.
	FormatRaw Format = "raw"
)

// ErrInvalidFormat signals an unsupported format.
var ErrInvalidFormat = errors.New("jobs: invalid format")

// FollowCommand tails a job's logs via SSE.
type FollowCommand struct {
	Client  stream.Client
	BaseURL *url.URL
	JobID   string
	Format  Format
	Output  io.Writer
}

// Run executes the follow command.
func (c FollowCommand) Run(ctx context.Context) error {
	format := c.Format
	if format == "" {
		format = FormatStructured
	}
	if format != FormatStructured && format != FormatRaw {
		return ErrInvalidFormat
	}
	if strings.TrimSpace(c.JobID) == "" {
		return errors.New("jobs: job id required")
	}
	if c.BaseURL == nil {
		return errors.New("jobs: base url required")
	}
	writer := c.Output
	if writer == nil {
		writer = io.Discard
	}

	endpoint, err := url.JoinPath(c.BaseURL.String(), "v2", "jobs", url.PathEscape(strings.TrimSpace(c.JobID)), "logs", "stream")
	if err != nil {
		return fmt.Errorf("jobs: build endpoint: %w", err)
	}

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "", "log":
			if len(evt.Data) == 0 {
				return nil
			}
			var payload logEvent
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("jobs: decode log event: %w", err)
			}
			printLog(writer, format, payload)
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown events
		}
		return nil
	}

	return c.Client.Stream(ctx, endpoint, handler)
}

type logEvent struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
}

func printLog(w io.Writer, format Format, evt logEvent) {
	line := strings.TrimRight(evt.Line, "\r\n")
	switch format {
	case FormatRaw:
		_, _ = fmt.Fprintf(w, "%s\n", line)
	default:
		timestamp := strings.TrimSpace(evt.Timestamp)
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}
		streamName := strings.TrimSpace(evt.Stream)
		if streamName == "" {
			streamName = "stdout"
		}
		_, _ = fmt.Fprintf(w, "%s %s %s\n", timestamp, streamName, line)
	}
}
