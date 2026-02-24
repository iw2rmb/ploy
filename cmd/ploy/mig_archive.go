// mod_archive.go implements the 'ploy mig archive' command handler.
//
// This command archives a mig project:
// - ploy mig archive <mig-id|name>
// - Refuses when any jobs for that mig are currently running.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigArchive implements 'ploy mig archive <mig-id|name>'.
func handleMigArchive(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigArchiveUsage(stderr)
		return nil
	}

	// Require mig ID or name as positional arg.
	if len(args) == 0 {
		printMigArchiveUsage(stderr)
		return fmt.Errorf("mig id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mig archive command.
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

// printMigArchiveUsage prints usage for the mig archive command.
func printMigArchiveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig archive <mig-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Archives a mig project. Refuses if any jobs for that mig are running.")
}
