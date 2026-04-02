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

// handleRunStart implements `ploy run start <run-id>`.
// Starts execution for pending repos in a batch run.
func handleRunStart(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunStartUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("run start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := parseFlagSet(fs, args, func() { printRunStartUsage(stderr) }); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunStartUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	cmd := runcmd.StartCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Run %s: started %d repo(s), %d already done, %d pending\n",
		result.RunID, result.Started, result.AlreadyDone, result.Pending)

	return nil
}

func printRunStartUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run start <run-id>")
}
