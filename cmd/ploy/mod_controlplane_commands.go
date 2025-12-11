package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleModCancel(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runFlag := fs.String("run", "", "mods run id to cancel")
	reason := fs.String("reason", "", "optional reason for cancellation")
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	if strings.TrimSpace(*runFlag) == "" {
		printModUsage(stderr)
		return errors.New("run id required")
	}
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.CancelCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(strings.TrimSpace(*runFlag)),
		Reason:  strings.TrimSpace(*reason),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}

func handleModResume(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod resume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.ResumeCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}

func handleModArtifacts(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.ArtifactsCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}

func handleModDiffs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod diffs", flag.ContinueOnError)
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
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if runIDArg == "" {
		if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
			printModUsage(stderr)
			return errors.New("run id required")
		}
		runIDArg = strings.TrimSpace(rest[0])
	}
	runID := domaintypes.RunID(runIDArg) // Convert to domain type
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.DiffsCommand{BaseURL: base, Client: httpClient, RunID: runID, Output: stderr, Download: *download, SavePath: strings.TrimSpace(*savePath)}
	return cmd.Run(ctx)
}
