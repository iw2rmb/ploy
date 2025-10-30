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
	"github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/stream"
)

func handleMods(args []string, stderr io.Writer) error {
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
	if err := fs.Parse(args); err != nil {
		printModsUsage(stderr)
		return err
	}

	ticketArgs := fs.Args()
	if len(ticketArgs) == 0 {
		printModsUsage(stderr)
		return errors.New("ticket required")
	}
	ticket := strings.TrimSpace(ticketArgs[0])
	if ticket == "" {
		printModsUsage(stderr)
		return errors.New("ticket required")
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
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	cmd := mods.LogsCommand{
		Ticket: ticket,
		Format: mods.Format(strings.ToLower(strings.TrimSpace(*format))),
		Output: stderr,
		Client: stream.Client{
			HTTPClient:   httpClient,
			MaxRetries:   *maxRetries,
			RetryBackoff: *retryWait,
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

func handleJobs(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printJobsUsage(stderr)
		return errors.New("jobs subcommand required")
	}
	switch args[0] {
	case "follow":
		return handleJobsFollow(args[1:], stderr)
	case "ls":
		return handleJobsList(args[1:], stderr)
	case "inspect":
		return handleJobsInspect(args[1:], stderr)
	case "retry":
		return handleJobsRetry(args[1:], stderr)
	default:
		printJobsUsage(stderr)
		return fmt.Errorf("unknown jobs subcommand %q", args[0])
	}
}

func handleJobsFollow(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("jobs follow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", string(jobs.FormatStructured), "output format (raw|structured)")
	maxRetries := fs.Int("max-retries", 3, "max reconnect attempts (-1 for unlimited)")
	retryWait := fs.Duration("retry-wait", 500*time.Millisecond, "wait duration between reconnect attempts")
	if err := fs.Parse(args); err != nil {
		printJobsUsage(stderr)
		return err
	}

	jobArgs := fs.Args()
	if len(jobArgs) == 0 {
		printJobsUsage(stderr)
		return errors.New("job id required")
	}
	jobID := strings.TrimSpace(jobArgs[0])
	if jobID == "" {
		printJobsUsage(stderr)
		return errors.New("job id required")
	}
	if *maxRetries < -1 {
		printJobsUsage(stderr)
		return fmt.Errorf("max retries must be >= -1")
	}
	if *retryWait < 0 {
		printJobsUsage(stderr)
		return fmt.Errorf("retry wait must be non-negative")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	cmd := jobs.FollowCommand{
		JobID:  jobID,
		Format: jobs.Format(strings.ToLower(strings.TrimSpace(*format))),
		Output: stderr,
		Client: stream.Client{
			HTTPClient:   httpClient,
			MaxRetries:   *maxRetries,
			RetryBackoff: *retryWait,
		},
		BaseURL: base,
	}
	if err := cmd.Run(ctx); err != nil {
		if errors.Is(err, jobs.ErrInvalidFormat) {
			printJobsUsage(stderr)
		}
		return err
	}
	return nil
}

func handleJobsList(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("jobs ls", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    ticket := fs.String("ticket", "", "mods ticket id to scope the list")
    if err := fs.Parse(args); err != nil {
        printJobsUsage(stderr)
        return err
    }
    if strings.TrimSpace(*ticket) == "" {
        printJobsUsage(stderr)
        return errors.New("ticket required")
    }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    cmd := jobs.ListCommand{BaseURL: base, Client: httpClient, Ticket: strings.TrimSpace(*ticket), Output: stderr}
    return cmd.Run(ctx)
}

func handleJobsInspect(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("jobs inspect", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    ticket := fs.String("ticket", "", "mods ticket id that owns the job")
    if err := fs.Parse(args); err != nil {
        printJobsUsage(stderr)
        return err
    }
    rest := fs.Args()
    if len(rest) == 0 || strings.TrimSpace(*ticket) == "" {
        printJobsUsage(stderr)
        return errors.New("ticket and job id required")
    }
    jobID := strings.TrimSpace(rest[0])
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    cmd := jobs.InspectCommand{BaseURL: base, Client: httpClient, Ticket: strings.TrimSpace(*ticket), JobID: jobID, Output: stderr}
    return cmd.Run(ctx)
}

func handleJobsRetry(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("jobs retry", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    ticket := fs.String("ticket", "", "mods ticket id that owns the job")
    if err := fs.Parse(args); err != nil {
        printJobsUsage(stderr)
        return err
    }
    rest := fs.Args()
    if len(rest) == 0 || strings.TrimSpace(*ticket) == "" {
        printJobsUsage(stderr)
        return errors.New("ticket and job id required")
    }
    jobID := strings.TrimSpace(rest[0])
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    cmd := jobs.RetryCommand{BaseURL: base, Client: httpClient, Ticket: strings.TrimSpace(*ticket), JobID: jobID, Output: stderr}
    return cmd.Run(ctx)
}
