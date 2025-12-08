package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	runscli "github.com/iw2rmb/ploy/internal/cli/runs"
	"github.com/iw2rmb/ploy/internal/cli/stream"
)

func handleMods(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printModsUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printModsUsage(stderr)
		return errors.New("mods subcommand required")
	}
	switch args[0] {
	case "logs":
		return handleModsLogs(args[1:], stderr)
	case "events":
		// Alias for logs; reserved for future structured events.
		return handleModsLogs(args[1:], stderr)
	default:
		printModsUsage(stderr)
		return fmt.Errorf("unknown mods subcommand %q", args[0])
	}
}

func handleModsLogs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mods logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", string(mods.FormatStructured), "output format (raw|structured)")
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	retryWait := fs.Duration("retry-wait", time.Second, "wait duration between reconnect attempts")
	idle := fs.Duration("idle-timeout", 45*time.Second, "cancel if no events arrive within this duration (0=off)")
	overall := fs.Duration("timeout", 0, "overall timeout for the stream (0=off)")
	if err := fs.Parse(args); err != nil {
		printModsUsage(stderr)
		return err
	}

	runIDArgs := fs.Args()
	if len(runIDArgs) == 0 {
		printModsUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(runIDArgs[0])
	if runID == "" {
		printModsUsage(stderr)
		return errors.New("run id required")
	}
	if *maxRetries < -1 {
		printModsUsage(stderr)
		return fmt.Errorf("max retries must be >= -1")
	}
	if *retryWait < 0 {
		printModsUsage(stderr)
		return fmt.Errorf("retry wait must be non-negative")
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

	cmd := mods.LogsCommand{
		RunID:  runID,
		Format: mods.Format(strings.ToLower(strings.TrimSpace(*format))),
		Output: stderr,
		Client: stream.Client{
			HTTPClient:   cloneForStream(httpClient),
			MaxRetries:   *maxRetries,
			RetryBackoff: *retryWait,
			IdleTimeout:  *idle,
		},
		BaseURL: base,
	}
	if err := cmd.Run(ctx); err != nil {
		if errors.Is(err, mods.ErrInvalidFormat) {
			printModsUsage(stderr)
		}
		return err
	}
	return nil
}

func handleRuns(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printRunsUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printRunsUsage(stderr)
		return errors.New("runs subcommand required")
	}
	switch args[0] {
	case "follow":
		return handleRunsFollow(args[1:], stderr)
	case "inspect":
		return handleRunsInspect(args[1:], stderr)
	default:
		printRunsUsage(stderr)
		return fmt.Errorf("unknown runs subcommand %q", args[0])
	}
}

func handleRunsFollow(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("runs follow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", string(runscli.FormatStructured), "output format (raw|structured)")
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	retryWait := fs.Duration("retry-wait", 500*time.Millisecond, "wait duration between reconnect attempts")
	idle := fs.Duration("idle-timeout", 45*time.Second, "cancel if no events arrive within this duration (0=off)")
	overall := fs.Duration("timeout", 0, "overall timeout for the stream (0=off)")
	if err := fs.Parse(args); err != nil {
		printRunsUsage(stderr)
		return err
	}

	jobArgs := fs.Args()
	if len(jobArgs) == 0 {
		printRunsUsage(stderr)
		return errors.New("job id required")
	}
	jobID := strings.TrimSpace(jobArgs[0])
	if jobID == "" {
		printRunsUsage(stderr)
		return errors.New("job id required")
	}
	if *maxRetries < -1 {
		printRunsUsage(stderr)
		return fmt.Errorf("max retries must be >= -1")
	}
	if *retryWait < 0 {
		printRunsUsage(stderr)
		return fmt.Errorf("retry wait must be non-negative")
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

	cmd := runscli.FollowCommand{
		JobID:  jobID,
		Format: runscli.Format(strings.ToLower(strings.TrimSpace(*format))),
		Output: stderr,
		Client: stream.Client{
			HTTPClient:   cloneForStream(httpClient),
			MaxRetries:   *maxRetries,
			RetryBackoff: *retryWait,
			IdleTimeout:  *idle,
		},
		BaseURL: base,
	}
	if err := cmd.Run(ctx); err != nil {
		if errors.Is(err, runscli.ErrInvalidFormat) {
			printRunsUsage(stderr)
		}
		return err
	}
	return nil
}

func handleRunsInspect(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("runs inspect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printRunsUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		printRunsUsage(stderr)
		return errors.New("job id required")
	}
	jobID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := runscli.InspectCommand{BaseURL: base, Client: httpClient, JobID: jobID, Output: stderr}
	return cmd.Run(ctx)
}
