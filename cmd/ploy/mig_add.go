// mod_add.go implements the 'ploy mig add' command handler.
//
// This command creates a mig project:
// - ploy mig add --name <name> [--spec <path|->]
// - Creates a mig with unique name.
// - If --spec is provided, creates initial spec row and sets migs.spec_id.
// - Prints mod_id and name; if --spec is provided, also prints spec_id.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/migs"
)

// handleMigAdd implements 'ploy mig add --name <name> [--spec <path|->]'.
func handleMigAdd(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigAddUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mig add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "Unique name for the mig (required)")
	specFile := fs.String("spec", "", "Path to YAML/JSON spec file (use '-' for stdin)")

	if err := fs.Parse(args); err != nil {
		printMigAddUsage(stderr)
		return err
	}

	// Validate required flags.
	if *name == "" {
		printMigAddUsage(stderr)
		return fmt.Errorf("--name is required")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Load spec from file or stdin if provided.
	// Uses loadSpec from run_submit.go which handles YAML/JSON parsing.
	var specData *json.RawMessage
	if *specFile != "" {
		data, err := loadSpec(ctx, base, httpClient, *specFile)
		if err != nil {
			return fmt.Errorf("load spec: %w", err)
		}
		specData = &data
	}

	// Execute mig add command.
	cmd := migs.AddMigCommand{
		Client:  httpClient,
		BaseURL: base,
		Name:    *name,
		Spec:    specData,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Print result.
	if result.SpecID != nil {
		_, _ = fmt.Fprintf(stderr, "Mod created: %s (name: %s, spec_id: %s)\n", result.ID.String(), result.Name, result.SpecID.String())
	} else {
		_, _ = fmt.Fprintf(stderr, "Mod created: %s (name: %s)\n", result.ID.String(), result.Name)
	}

	return nil
}

// printMigAddUsage prints usage for the mig add command.
func printMigAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig add --name <name> [--spec <path|->]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --name <name>    Unique name for the mig (required)")
	_, _ = fmt.Fprintln(w, "  --spec <path|->  Path to YAML/JSON spec file (use '-' for stdin)")
}
