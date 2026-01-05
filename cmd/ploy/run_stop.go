package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleRunStop implements `ploy run stop <run-id>`.
// Stops a run by calling the v1 cancel endpoint (Queued/Running repos -> Cancelled).
func handleRunStop(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("run stop", flag.ContinueOnError)
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

	cmd := runcmd.StopCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}

	summary, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Run %s stopped (status: %s)\n", summary.ID, summary.Status)
	if summary.Counts != nil && summary.Counts.Cancelled > 0 {
		_, _ = fmt.Fprintf(stderr, "Cancelled %d pending repo(s)\n", summary.Counts.Cancelled)
	}

	return nil
}
