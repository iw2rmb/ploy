package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	jsonOut := fs.Bool("json", false, "print machine-readable JSON report")

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
	token, err := resolveControlPlaneToken()
	if err != nil {
		return err
	}
	report, err := runcmd.GetRunReportCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	if *jsonOut {
		return runcmd.RenderRunReportJSON(stderr, report)
	}

	if err := runcmd.RenderRunReportText(stderr, report, runcmd.TextRenderOptions{
		EnableOSC8: runStatusSupportsOSC8(stderr),
		AuthToken:  token,
	}); err != nil {
		return err
	}

	return nil
}

func runStatusSupportsOSC8(w io.Writer) bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || strings.EqualFold(term, "dumb") {
		return false
	}

	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
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
