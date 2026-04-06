package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
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
	case "patch":
		// Patch command: downloads a diff artifact without applying it.
		return handleRunPatch(args[1:], stderr)
	default:
		printRunUsage(stderr)
		return fmt.Errorf("unknown run subcommand %q", args[0])
	}
}

func handleRunStatus(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printRunStatusUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("run status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "print machine-readable JSON report with links and per-job artifacts")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRunStatusUsage(stderr)
			return nil
		}
		printRunStatusUsage(stderr)
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printRunStatusUsage(stderr)
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
		BaseURL:    base,
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

func printRunStatusUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run status [--json] <run-id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --json   Print machine-readable JSON report with links and per-job artifacts")
}
