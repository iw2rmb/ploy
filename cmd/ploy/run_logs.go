package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	logstream "github.com/iw2rmb/ploy/internal/stream"
)

func handleRunLogs(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunLogsUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("run logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	idle := fs.Duration("idle-timeout", 45*time.Second, "cancel if no events arrive within this duration (0=off)")
	overall := fs.Duration("timeout", 0, "overall timeout for the stream (0=off)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRunLogsUsage(stderr)
			return nil
		}
		printRunLogsUsage(stderr)
		return err
	}

	runIDArgs := fs.Args()
	if len(runIDArgs) == 0 {
		printRunLogsUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(runIDArgs[0])
	if runID == "" {
		printRunLogsUsage(stderr)
		return errors.New("run id required")
	}
	if *maxRetries < -1 {
		printRunLogsUsage(stderr)
		return fmt.Errorf("max retries must be >= -1")
	}

	ctx := context.Background()
	if *overall > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *overall)
		defer cancel()
	}
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	endpoint := base.JoinPath("v1", "runs", domaintypes.RunID(runID).String(), "logs")

	client := stream.Client{
		HTTPClient:  cloneForStream(httpClient),
		MaxRetries:  *maxRetries,
		IdleTimeout: *idle,
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
			_, _ = fmt.Fprintf(stderr, "%s [run] state=%s\n", ts, payload.State)
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
				_, _ = fmt.Fprintf(stderr, "%s [stage] %s %s\n", ts, ctx.String(), line)
			} else {
				_, _ = fmt.Fprintf(stderr, "%s [stage] %s\n", ts, line)
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

func printRunLogsUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run logs [--max-retries <n>] [--idle-timeout <duration>] [--timeout <duration>] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Streams run lifecycle events (run state changes and stage transitions).")
	_, _ = fmt.Fprintln(w, "For container logs, use: ploy job log --follow <job-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>           Max reconnect attempts (-1 for unlimited, default: 3)")
	_, _ = fmt.Fprintln(w, "  --idle-timeout <duration>   Cancel if no events arrive (0=off, default: 45s)")
	_, _ = fmt.Fprintln(w, "  --timeout <duration>        Overall stream timeout (0=off)")
}
