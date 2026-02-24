// mod_remove.go implements the 'ploy mod remove' command handler.
//
// This command deletes a mod project:
// - ploy mod remove <mod-id|name>
// - Refuses if the mod has any runs.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleModRemove implements 'ploy mod remove <mod-id|name>'.
func handleModRemove(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRemoveUsage(stderr)
		return nil
	}

	// Require mod ID or name as positional arg.
	if len(args) == 0 {
		printModRemoveUsage(stderr)
		return fmt.Errorf("mod id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mod remove command.
	cmd := mods.RemoveModCommand{
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

// printModRemoveUsage prints usage for the mod remove command.
func printModRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod remove <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Deletes a mod project. Refuses if the mod has any runs.")
}
