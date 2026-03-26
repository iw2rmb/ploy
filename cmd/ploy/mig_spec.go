// mod_spec.go implements the 'ploy mig spec' command handler.
//
// This command sets a mig's spec:
// - ploy mig spec set <mig-id|name> <path|->
// - Stores the parsed spec JSON (from YAML/JSON input).
// - Validates spec shape.
// - Inserts a new specs row and updates migs.spec_id to that new spec_id.
// - Returns spec_id.
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigSpec routes mig spec subcommands.
func handleMigSpec(args []string, stderr io.Writer) error {
	// Handle help flag or empty args.
	if wantsHelp(args) || len(args) == 0 {
		printMigSpecUsage(stderr)
		if len(args) == 0 {
			return fmt.Errorf("mig spec subcommand required")
		}
		return nil
	}

	switch args[0] {
	case "set":
		return handleMigSpecSet(args[1:], stderr)
	default:
		printMigSpecUsage(stderr)
		return fmt.Errorf("unknown mig spec subcommand %q", args[0])
	}
}

// handleMigSpecSet implements 'ploy mig spec set <mig-id|name> <path|->'.
func handleMigSpecSet(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigSpecSetUsage(stderr)
		return nil
	}

	// Require mig ID/name and spec path as positional args.
	if len(args) < 2 {
		printMigSpecSetUsage(stderr)
		return fmt.Errorf("mig id/name and spec path are required")
	}
	modRef := args[0]
	specPath := args[1]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Load spec from file or stdin using shared loadSpec function.
	specData, err := loadSpec(ctx, base, httpClient, specPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	// Execute set mig spec command.
	cmd := migs.SetModSpecCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
		Spec:    specData,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Spec set: %s (created_at: %s)\n", result.ID.String(), result.CreatedAt.Format(time.RFC3339))
	return nil
}

// printMigSpecUsage prints usage for the mig spec command.
func printMigSpecUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig spec <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  set <mig-id|name> <path|->  Set the mig's spec from a file or stdin")
}

// printMigSpecSetUsage prints usage for the mig spec set command.
func printMigSpecSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig spec set <mig-id|name> <path|->")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Sets the mig's spec from a YAML/JSON file (use '-' for stdin).")
	_, _ = fmt.Fprintln(w, "Creates a new spec row and updates the mig's current spec.")
}
