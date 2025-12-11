// mod_run_batch.go implements batch run lifecycle CLI commands (list/stop/start).
//
// This file provides CLI handlers for managing batch runs as a whole, complementing
// the repo-level operations in mod_run_repo.go. Batch commands delegate to the
// internal/cli/mods batch client for HTTP communication with the control plane.
//
// Command structure:
//   - ploy mod run list [--limit N] [--offset N]
//   - ploy mod run stop <run-name>
//   - ploy mod run start <run-name>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleModRunList implements `ploy mod run list [--limit N] [--offset N]`.
// Lists batch runs with pagination, showing ID, name, status, and repo counts.
func handleModRunList(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 50, "Max number of runs to return (1-100)")
	offset := fs.Int("offset", 0, "Number of runs to skip")

	if err := fs.Parse(args); err != nil {
		printModRunListUsage(stderr)
		return err
	}

	// Validate pagination parameters.
	if *limit < 1 || *limit > 100 {
		printModRunListUsage(stderr)
		return errors.New("limit must be between 1 and 100")
	}
	if *offset < 0 {
		printModRunListUsage(stderr)
		return errors.New("offset must be non-negative")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute the list command using the batch client.
	cmd := mods.ListBatchesCommand{
		Client:  httpClient,
		BaseURL: base,
		Limit:   int32(*limit),
		Offset:  int32(*offset),
	}

	batches, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	if len(batches) == 0 {
		_, _ = fmt.Fprintln(stderr, "No batch runs found.")
		return nil
	}

	// Print results in tabular format.
	tw := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tREPOS\tDERIVED STATUS")
	for _, b := range batches {
		name := "-"
		if b.Name != nil && *b.Name != "" {
			name = *b.Name
		}
		repos := "-"
		derived := "-"
		if b.Counts != nil {
			// Format repo counts as: succeeded/total (e.g., "3/5").
			repos = fmt.Sprintf("%d/%d", b.Counts.Succeeded, b.Counts.Total)
			derived = b.Counts.DerivedStatus
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			b.ID, name, b.Status, repos, derived)
	}
	_ = tw.Flush()
	return nil
}

// printModRunListUsage renders help for mod run list.
func printModRunListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run list [--limit N] [--offset N]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists batch runs with pagination.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --limit N   Max runs to return (1-100, default 50)")
	_, _ = fmt.Fprintln(w, "  --offset N  Number of runs to skip (default 0)")
}

// NOTE: The `ploy mod run status` command has been removed.
// Run-level status is now exposed via `ploy run status <run-id>`, which
// reuses the richer status output previously implemented here.

// handleModRunStop implements `ploy mod run stop <run-name>`.
// Stops a batch run and cancels all pending repos.
func handleModRunStop(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run stop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		printModRunStopUsage(stderr)
		return err
	}

	// Extract positional batch ID.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModRunStopUsage(stderr)
		return errors.New("run-name required")
	}
	batchID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute the stop command using the batch client.
	cmd := mods.StopBatchCommand{
		Client:  httpClient,
		BaseURL: base,
		BatchID: domaintypes.RunID(batchID), // Convert to domain type
	}

	batch, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Batch run %s stopped (status: %s)\n", batch.ID, batch.Status)
	if batch.Counts != nil && batch.Counts.Cancelled > 0 {
		_, _ = fmt.Fprintf(stderr, "Cancelled %d pending repo(s)\n", batch.Counts.Cancelled)
	}

	return nil
}

// printModRunStopUsage renders help for mod run stop.
func printModRunStopUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run stop <run-name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Stops a batch run and cancels all pending repos.")
}

// handleModRunStart implements `ploy mod run start <run-name>`.
// Starts execution for pending repos in a batch run.
func handleModRunStart(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		printModRunStartUsage(stderr)
		return err
	}

	// Extract positional batch ID.
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModRunStartUsage(stderr)
		return errors.New("run-name required")
	}
	batchID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute the start command using the batch client.
	cmd := mods.StartBatchCommand{
		Client:  httpClient,
		BaseURL: base,
		BatchID: domaintypes.RunID(batchID), // Convert to domain type
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Batch run %s: started %d repo(s), %d already done, %d pending\n",
		result.RunID, result.Started, result.AlreadyDone, result.Pending)

	return nil
}

// printModRunStartUsage renders help for mod run start.
func printModRunStartUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run start <run-name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Starts execution for pending repos in a batch run.")
}
