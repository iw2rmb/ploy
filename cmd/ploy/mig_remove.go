// mod_remove.go implements the 'ploy mig remove' command handler.
//
// This command deletes a mig project:
// - ploy mig remove <mig-id|name>
// - Refuses if the mig has any runs.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigRemove implements 'ploy mig remove <mig-id|name>'.
func handleMigRemove(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigRemoveUsage(stderr)
		return nil
	}

	// Require mig ID or name as positional arg.
	if len(args) == 0 {
		printMigRemoveUsage(stderr)
		return fmt.Errorf("mig id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mig remove command.
	cmd := migs.RemoveModCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}

	if err := cmd.Run(ctx); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Mod deleted: %s\n", modRef)
	return nil
}

// printMigRemoveUsage prints usage for the mig remove command.
func printMigRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig remove <mig-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Deletes a mig project. Refuses if the mig has any runs.")
}
