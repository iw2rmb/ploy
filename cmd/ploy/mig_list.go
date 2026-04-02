// mig_list.go implements the 'ploy mig list' command handler.
//
// This command lists mig projects:
// - ploy mig list
// - Lists migs: ID, NAME, CREATED_AT, ARCHIVED.
package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/migs"
)

// handleMigList implements 'ploy mig list'.
func handleMigList(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigListUsage(stderr)
		return nil
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mig list command.
	cmd := migs.ListMigsCommand{
		Client:  httpClient,
		BaseURL: base,
		Limit:   100,
	}

	results, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintln(stderr, "No migs found.")
		return nil
	}

	// Print results in tabular format.
	w := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCREATED_AT\tARCHIVED")
	for _, mig := range results {
		archived := "-"
		if mig.Archived {
			archived = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", mig.ID.String(), mig.Name, mig.CreatedAt.Format(time.RFC3339), archived)
	}
	_ = w.Flush()

	return nil
}

// printMigListUsage prints usage for the mig list command.
func printMigListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig list")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists mig projects: ID, NAME, CREATED_AT, ARCHIVED.")
}
