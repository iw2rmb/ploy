// mod_unarchive.go implements the 'ploy mod unarchive' command handler.
//
// This command unarchives a mod project:
// - ploy mod unarchive <mod-id|name>
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleModUnarchive implements 'ploy mod unarchive <mod-id|name>'.
func handleModUnarchive(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModUnarchiveUsage(stderr)
		return nil
	}

	// Require mod ID or name as positional arg.
	if len(args) == 0 {
		printModUnarchiveUsage(stderr)
		return fmt.Errorf("mod id or name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mod unarchive command.
	cmd := mods.UnarchiveMigCommand{
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

// printModUnarchiveUsage prints usage for the mod unarchive command.
func printModUnarchiveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod unarchive <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Unarchives a mod project.")
}
