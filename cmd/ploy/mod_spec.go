// mod_spec.go implements the 'ploy mod spec' command handler.
//
// Per roadmap/v1/cli.md:53-60, this command sets a mod's spec:
// - ploy mod spec set <mod-id|name> <path|->
// - Stores the parsed spec JSON (from YAML/JSON input).
// - Validates spec shape.
// - Inserts a new specs row and updates mods.spec_id to that new spec_id.
// - Returns spec_id.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
)

// handleModSpec routes mod spec subcommands.
func handleModSpec(args []string, stderr io.Writer) error {
	// Handle help flag or empty args.
	if wantsHelp(args) || len(args) == 0 {
		printModSpecUsage(stderr)
		if len(args) == 0 {
			return fmt.Errorf("mod spec subcommand required")
		}
		return nil
	}

	switch args[0] {
	case "set":
		return handleModSpecSet(args[1:], stderr)
	default:
		printModSpecUsage(stderr)
		return fmt.Errorf("unknown mod spec subcommand %q", args[0])
	}
}

// handleModSpecSet implements 'ploy mod spec set <mod-id|name> <path|->'.
func handleModSpecSet(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModSpecSetUsage(stderr)
		return nil
	}

	// Require mod ID/name and spec path as positional args.
	if len(args) < 2 {
		printModSpecSetUsage(stderr)
		return fmt.Errorf("mod id/name and spec path are required")
	}
	modRef := args[0]
	specPath := args[1]

	// Load spec from file or stdin using shared loadSpec function.
	specData, err := loadSpec(specPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

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

	// Execute set mod spec command.
	cmd := mods.SetModSpecCommand{
		Client:  httpClient,
		BaseURL: base,
		ModID:   modID,
		Spec:    specData,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Spec set: %s (created_at: %s)\n", result.ID, result.CreatedAt)
	return nil
}

// printModSpecUsage prints usage for the mod spec command.
func printModSpecUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod spec <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  set <mod-id|name> <path|->  Set the mod's spec from a file or stdin")
}

// printModSpecSetUsage prints usage for the mod spec set command.
func printModSpecSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod spec set <mod-id|name> <path|->")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Sets the mod's spec from a YAML/JSON file (use '-' for stdin).")
	_, _ = fmt.Fprintln(w, "Creates a new spec row and updates the mod's current spec.")
}
