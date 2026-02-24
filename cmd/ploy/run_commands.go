package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/migs"
	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleRun(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printRunUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printRunUsage(stderr)
		return errors.New("run subcommand required")
	}

	// Check if the first argument is a flag (starts with "-").
	// When flags are provided directly to `ploy run`, route to run submit.
	// This implements: ploy run --repo ... --base-ref ... --target-ref ... --spec ...
	if strings.HasPrefix(args[0], "-") {
		return handleRunSubmit(args, stderr)
	}

	switch args[0] {
	case "list":
		return handleRunList(args[1:], stderr)
	case "cancel":
		return handleRunCancel(args[1:], stderr)
	case "start":
		return handleRunStart(args[1:], stderr)
	case "stop":
		return handleRunStop(args[1:], stderr)
	case "status":
		return handleRunStatus(args[1:], stderr)
	case "logs":
		return handleRunLogs(args[1:], stderr)
	case "diff":
		return handleRunDiff(args[1:], stderr)
	case "pull":
		// Pull command: pulls diffs from a specific run into the current repo.
		return handleRunPull(args[1:], stderr)
	default:
		printRunUsage(stderr)
		return fmt.Errorf("unknown run subcommand %q", args[0])
	}
}

func handleRunStatus(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("run status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		printRunUsage(stderr)
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := runcmd.GetStatusCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}

	summary, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Rich summary output (previously used by `ploy mig run status`).
	_, _ = fmt.Fprintf(stderr, "Run: %s\n", summary.ID)
	_, _ = fmt.Fprintf(stderr, "Status: %s\n", summary.Status)
	_, _ = fmt.Fprintf(stderr, "Mod ID: %s\n", summary.MigID)
	_, _ = fmt.Fprintf(stderr, "Spec ID: %s\n", summary.SpecID)
	if summary.CreatedBy != nil && *summary.CreatedBy != "" {
		_, _ = fmt.Fprintf(stderr, "Created By: %s\n", *summary.CreatedBy)
	}
	_, _ = fmt.Fprintf(stderr, "Created At: %s\n", summary.CreatedAt.Format("2006-01-02 15:04:05"))
	if summary.StartedAt != nil {
		_, _ = fmt.Fprintf(stderr, "Started At: %s\n", summary.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if summary.FinishedAt != nil {
		_, _ = fmt.Fprintf(stderr, "Finished At: %s\n", summary.FinishedAt.Format("2006-01-02 15:04:05"))
	}

	if summary.Counts != nil {
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "Repo Counts:")
		_, _ = fmt.Fprintf(stderr, "  Total:     %d\n", summary.Counts.Total)
		_, _ = fmt.Fprintf(stderr, "  Queued:    %d\n", summary.Counts.Queued)
		_, _ = fmt.Fprintf(stderr, "  Running:   %d\n", summary.Counts.Running)
		_, _ = fmt.Fprintf(stderr, "  Success:   %d\n", summary.Counts.Success)
		_, _ = fmt.Fprintf(stderr, "  Fail:      %d\n", summary.Counts.Fail)
		_, _ = fmt.Fprintf(stderr, "  Cancelled: %d\n", summary.Counts.Cancelled)
		_, _ = fmt.Fprintf(stderr, "  Derived:   %s\n", summary.Counts.DerivedStatus)
	}

	return nil
}

func handleRunLogs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("run logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", string(logs.FormatStructured), "output format (raw|structured)")
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	idle := fs.Duration("idle-timeout", 45*time.Second, "cancel if no events arrive within this duration (0=off)")
	overall := fs.Duration("timeout", 0, "overall timeout for the stream (0=off)")
	if err := fs.Parse(args); err != nil {
		printRunUsage(stderr)
		return err
	}

	runIDArgs := fs.Args()
	if len(runIDArgs) == 0 {
		printRunUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(runIDArgs[0])
	if runID == "" {
		printRunUsage(stderr)
		return errors.New("run id required")
	}
	if *maxRetries < -1 {
		printRunUsage(stderr)
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

	cmd := migs.LogsCommand{
		RunID:  domaintypes.RunID(runID),
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
		if errors.Is(err, migs.ErrInvalidFormat) {
			printRunUsage(stderr)
		}
		return err
	}
	return nil
}
