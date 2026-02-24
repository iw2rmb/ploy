// mod_unarchive.go implements the 'ploy mig unarchive' command handler.
//
// This command unarchives a mig project:
// - ploy mig unarchive <mig-id|name>
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigUnarchive implements 'ploy mig unarchive <mig-id|name>'.
func handleMigUnarchive(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigUnarchiveUsage(stderr)
		return nil
	}

	// Require mig ID or name as positional arg.
	if len(args) == 0 {
		printMigUnarchiveUsage(stderr)
		return fmt.Errorf("mig id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mig unarchive command.
	cmd := migs.UnarchiveMigCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Mod unarchived: %s (name: %s)\n", result.ID.String(), result.Name)
	return nil
}

// printMigUnarchiveUsage prints usage for the mig unarchive command.
func printMigUnarchiveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig unarchive <mig-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Unarchives a mig project.")
}
