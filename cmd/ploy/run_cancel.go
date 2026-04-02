package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleRunCancel(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunCancelUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("run cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reason := fs.String("reason", "", "optional reason for cancellation")
	if err := parseFlagSet(fs, args, func() { printRunCancelUsage(stderr) }); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunCancelUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := runs.CancelCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Reason:  strings.TrimSpace(*reason),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}

func printRunCancelUsage(w io.Writer) {
	_, _ = io.WriteString(w, "Usage: ploy run cancel [--reason <text>] <run-id>\n")
}
