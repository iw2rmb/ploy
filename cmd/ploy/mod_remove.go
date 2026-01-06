// mod_remove.go implements the 'ploy mod remove' command handler.
//
// Per roadmap/v1/cli.md:37-40, this command deletes a mod project:
// - ploy mod remove <mod-id|name>
// - Refuses if the mod has any runs.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
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

	// Resolve mod reference to ID (supports name/ID resolution per roadmap/v1/cli.md:169-170).
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		ModRef:  modRef,
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod remove command.
	cmd := mods.RemoveModCommand{
		Client:  httpClient,
		BaseURL: base,
		ModID:   modID,
	}

	if err := cmd.Run(ctx); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Mod deleted: %s\n", modID)
	return nil
}

// printModRemoveUsage prints usage for the mod remove command.
func printModRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod remove <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Deletes a mod project. Refuses if the mod has any runs.")
}
