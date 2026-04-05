package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/jobs"
	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleJob(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printJobUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printJobUsage(stderr)
		return errors.New("job subcommand required")
	}

	switch args[0] {
	case "follow":
		return handleJobFollow(args[1:], stderr)
	default:
		printJobUsage(stderr)
		return fmt.Errorf("unknown job subcommand %q", args[0])
	}
}

func handleJobFollow(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printJobFollowUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("job follow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", string(logs.FormatStructured), "output format (raw|structured)")
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	idle := fs.Duration("idle-timeout", 45*time.Second, "cancel if no events arrive within this duration (0=off)")
	overall := fs.Duration("timeout", 0, "overall timeout for the stream (0=off)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printJobFollowUsage(stderr)
			return nil
		}
		printJobFollowUsage(stderr)
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printJobFollowUsage(stderr)
		return errors.New("job id required")
	}
	jobID := strings.TrimSpace(rest[0])
	if jobID == "" {
		printJobFollowUsage(stderr)
		return errors.New("job id required")
	}
	if *maxRetries < -1 {
		printJobFollowUsage(stderr)
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

	cmd := jobs.FollowCommand{
		JobID:  domaintypes.JobID(jobID),
		Format: logs.Format(strings.ToLower(strings.TrimSpace(*format))),
		Output: stderr,
		Client: stream.Client{
			HTTPClient:  cloneForStream(httpClient),
			MaxRetries:  *maxRetries,
			IdleTimeout: *idle,
		},
		BaseURL: base,
	}
	if err := cmd.Run(ctx); err != nil {
		if errors.Is(err, jobs.ErrInvalidFormat) {
			printJobFollowUsage(stderr)
		}
		return err
	}
	return nil
}

func printJobUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy job <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  follow      Stream job logs (SSE)")
}

func printJobFollowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy job follow [--format <raw|structured>] [--max-retries <n>] [--idle-timeout <duration>] [--timeout <duration>] <job-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --format <raw|structured>   Output format (default: structured)")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>           Max reconnect attempts (-1 for unlimited, default: 3)")
	_, _ = fmt.Fprintln(w, "  --idle-timeout <duration>   Cancel if no events arrive (0=off, default: 45s)")
	_, _ = fmt.Fprintln(w, "  --timeout <duration>        Overall stream timeout (0=off)")
}
