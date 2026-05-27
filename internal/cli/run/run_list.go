// run_list.go implements batch run ls CLI commands.
//
// This file provides CLI handlers for managing batch runs as a whole, complementing
// the repo-level operations in mig_run_repo.go. Batch commands delegate to the
// internal/cli/migs batch client for HTTP communication with the control plane.
//
// Command structure:
//   - ploy run ls [--limit N] [--offset N]
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/migs"
)

type ListOptions struct {
	Limit  int
	Offset int
	Output io.Writer
}

// RunList implements `ploy run ls [--limit N] [--offset N]`.
// Lists batch runs with pagination, showing ID, name, status, and repo counts.
func RunList(ctx context.Context, opts ListOptions) error {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	// Validate pagination parameters.
	if opts.Limit < 1 || opts.Limit > 100 {
		return errors.New("limit must be between 1 and 100")
	}
	if opts.Offset < 0 {
		return errors.New("offset must be non-negative")
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute the list command using the batch client.
	cmd := migs.ListBatchesCommand{
		Client:  httpClient,
		BaseURL: base,
		Limit:   int32(opts.Limit),
		Offset:  int32(opts.Offset),
	}

	batches, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	if len(batches) == 0 {
		_, _ = fmt.Fprintln(out, "No batch runs found.")
		return nil
	}

	// Print results in tabular format.
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
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
