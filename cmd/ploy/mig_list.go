// mod_list.go implements the 'ploy mig list' command handler.
//
// This command lists mod projects:
// - ploy mig list
// - Lists mods: ID, NAME, CREATED_AT, ARCHIVED.
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

	// Execute mod list command.
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
		_, _ = fmt.Fprintln(stderr, "No mods found.")
		return nil
	}

	// Print results in tabular format.
	w := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCREATED_AT\tARCHIVED")
	for _, mod := range results {
		archived := "-"
		if mod.Archived {
			archived = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", mod.ID.String(), mod.Name, mod.CreatedAt.Format(time.RFC3339), archived)
	}
	_ = w.Flush()

	return nil
}

// printMigListUsage prints usage for the mod list command.
func printMigListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig list")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists mod projects: ID, NAME, CREATED_AT, ARCHIVED.")
}
