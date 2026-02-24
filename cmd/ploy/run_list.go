// run_list.go implements batch run list CLI commands.
//
// This file provides CLI handlers for managing batch runs as a whole, complementing
// the repo-level operations in mod_run_repo.go. Batch commands delegate to the
// internal/cli/migs batch client for HTTP communication with the control plane.
//
// Command structure:
//   - ploy run list [--limit N] [--offset N]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/migs"
)

// handleRunList implements `ploy run list [--limit N] [--offset N]`.
// Lists batch runs with pagination, showing ID, name, status, and repo counts.
func handleRunList(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("run list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 50, "Max number of runs to return (1-100)")
	offset := fs.Int("offset", 0, "Number of runs to skip")

	if err := fs.Parse(args); err != nil {
		printRunListUsage(stderr)
		return err
	}

	// Validate pagination parameters.
	if *limit < 1 || *limit > 100 {
		printRunListUsage(stderr)
		return errors.New("limit must be between 1 and 100")
	}
	if *offset < 0 {
		printRunListUsage(stderr)
		return errors.New("offset must be non-negative")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute the list command using the batch client.
	cmd := migs.ListBatchesCommand{
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
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tMOD\tSPEC\tREPOS\tDERIVED STATUS")
	for _, b := range batches {
		repos := "-"
		derived := "-"
		if b.Counts != nil {
			// Format repo counts as: succeeded/total (e.g., "3/5").
			repos = fmt.Sprintf("%d/%d", b.Counts.Success, b.Counts.Total)
			derived = b.Counts.DerivedStatus
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.ID, b.Status, b.MigID, b.SpecID, repos, derived)
	}
	_ = tw.Flush()
	return nil
}

// printRunListUsage renders help for run list.
func printRunListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy run list [--limit N] [--offset N]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists batch runs with pagination.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --limit N   Max runs to return (1-100, default 50)")
	_, _ = fmt.Fprintln(w, "  --offset N  Number of runs to skip (default 0)")
}

// NOTE: The `ploy mig run status` command has been removed.
// Run-level status is now exposed via `ploy run status <run-id>`, which
// reuses the richer status output previously implemented here.
