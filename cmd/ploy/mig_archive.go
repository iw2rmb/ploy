// mod_archive.go implements the 'ploy mig archive' command handler.
//
// This command archives a mod project:
// - ploy mig archive <mod-id|name>
// - Refuses when any jobs for that mod are currently running.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigArchive implements 'ploy mig archive <mod-id|name>'.
func handleMigArchive(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigArchiveUsage(stderr)
		return nil
	}

	// Require mod ID or name as positional arg.
	if len(args) == 0 {
		printMigArchiveUsage(stderr)
		return fmt.Errorf("mod id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mod archive command.
	cmd := migs.ArchiveMigCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Mod archived: %s (name: %s)\n", result.ID.String(), result.Name)
	return nil
}

// printMigArchiveUsage prints usage for the mod archive command.
func printMigArchiveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig archive <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Archives a mod project. Refuses if any jobs for that mod are running.")
}
