// mod_archive.go implements the 'ploy mod archive' command handler.
//
// This command archives a mod project:
// - ploy mod archive <mod-id|name>
// - Refuses when any jobs for that mod are currently running.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
)

// handleModArchive implements 'ploy mod archive <mod-id|name>'.
func handleModArchive(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModArchiveUsage(stderr)
		return nil
	}

	// Require mod ID or name as positional arg.
	if len(args) == 0 {
		printModArchiveUsage(stderr)
		return fmt.Errorf("mod id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID (supports both name and ID).
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		ModRef:  modRef,
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod archive command.
	cmd := mods.ArchiveModCommand{
		Client:  httpClient,
		BaseURL: base,
		ModID:   modID,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Mod archived: %s (name: %s)\n", result.ID, result.Name)
	return nil
}

// printModArchiveUsage prints usage for the mod archive command.
func printModArchiveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod archive <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Archives a mod project. Refuses if any jobs for that mod are running.")
}
