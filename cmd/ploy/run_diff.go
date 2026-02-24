package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleRunDiff lists diffs for a run or downloads the newest patch.
// It replaces the legacy `ploy mig diffs` command with a run-scoped surface:
//
//	ploy run diffs [--download] [--output <file>] <run-id>
func handleRunDiff(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("run diffs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	download := fs.Bool("download", false, "download newest diff and print to stdout (gunzipped)")
	savePath := fs.String("output", "", "save newest diff to file (gunzipped)")

	// Allow both orders: flags before or after run id.
	var runIDArg string
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		runIDArg = strings.TrimSpace(args[0])
		args = args[1:]
	}

	if err := fs.Parse(args); err != nil {
		printRunUsage(stderr)
		return err
	}

	rest := fs.Args()
	if runIDArg == "" {
		if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
			printRunUsage(stderr)
			return errors.New("run id required")
		}
		runIDArg = strings.TrimSpace(rest[0])
	}

	runID := domaintypes.RunID(runIDArg)
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	cmd := runcmd.DiffsCommand{
		BaseURL:  base,
		Client:   httpClient,
		RunID:    runID,
		Output:   stderr,
		Download: *download,
		SavePath: strings.TrimSpace(*savePath),
	}
	return cmd.Run(ctx)
}
