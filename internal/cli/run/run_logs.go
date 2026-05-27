package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

type LogsOptions struct {
	RunID       string
	MaxRetries  int
	IdleTimeout time.Duration
	Timeout     time.Duration
	Output      io.Writer
}

func RunLogs(ctx context.Context, opts LogsOptions) error {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return errors.New("run id required")
	}
	if opts.MaxRetries < -1 {
		return fmt.Errorf("max retries must be >= -1")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	endpoint := base.JoinPath("v1", "runs", domaintypes.RunID(runID).String(), "logs")

	client := stream.Client{
		HTTPClient:  common.CloneForStream(httpClient),
		MaxRetries:  opts.MaxRetries,
		IdleTimeout: opts.IdleTimeout,
	}

	handler := func(evt stream.Event) error {
		switch strings.ToLower(evt.Type) {
		case "run":
			if len(evt.Data) == 0 {
				return nil
			}
			var payload struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return fmt.Errorf("run logs: decode run event: %w", err)
			}
			ts := time.Now().UTC().Format(time.RFC3339)
			_, _ = fmt.Fprintf(out, "%s [run] state=%s\n", ts, payload.State)
		case "stage":
			if len(evt.Data) == 0 {
				return nil
			}
			var rec logstream.LogRecord
			if err := json.Unmarshal(evt.Data, &rec); err != nil {
				return fmt.Errorf("run logs: decode stage event: %w", err)
			}
			ts := strings.TrimSpace(rec.Timestamp)
			if ts == "" {
				ts = time.Now().UTC().Format(time.RFC3339)
			}
			var ctx strings.Builder
			if !rec.JobID.IsZero() {
				ctx.WriteString("job=")
				ctx.WriteString(rec.JobID.String())
			}
			if !rec.JobType.IsZero() {
				if ctx.Len() > 0 {
					ctx.WriteByte(' ')
				}
				ctx.WriteString("type=")
				ctx.WriteString(rec.JobType.String())
			}
			line := strings.TrimRight(rec.Line, "\r\n")
			if ctx.Len() > 0 {
				_, _ = fmt.Fprintf(out, "%s [stage] %s %s\n", ts, ctx.String(), line)
			} else {
				_, _ = fmt.Fprintf(out, "%s [stage] %s\n", ts, line)
			}
		case "done", "complete", "completed":
			return stream.ErrDone
		default:
			// ignore unknown lifecycle event types
		}
		return nil
	}

	return client.Stream(ctx, endpoint.String(), handler)
}
