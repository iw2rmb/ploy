// mod_add.go implements the 'ploy mod add' command handler.
//
// This command creates a mod project:
// - ploy mod add --name <name> [--spec <path|->]
// - Creates a mod with unique name.
// - If --spec is provided, creates initial spec row and sets mods.spec_id.
// - Prints mod_id and name; if --spec is provided, also prints spec_id.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/cli/mods"
)

// handleModAdd implements 'ploy mod add --name <name> [--spec <path|->]'.
func handleModAdd(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModAddUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mod add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "Unique name for the mod (required)")
	specFile := fs.String("spec", "", "Path to YAML/JSON spec file (use '-' for stdin)")

	if err := fs.Parse(args); err != nil {
		printModAddUsage(stderr)
		return err
	}

	// Validate required flags.
	if *name == "" {
		printModAddUsage(stderr)
		return fmt.Errorf("--name is required")
	}

	// Load spec from file or stdin if provided.
	// Uses loadSpec from run_submit.go which handles YAML/JSON parsing.
	var specData *json.RawMessage
	if *specFile != "" {
		data, err := loadSpec(*specFile)
		if err != nil {
			return fmt.Errorf("load spec: %w", err)
		}
		specData = &data
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Execute mod add command.
	cmd := mods.AddModCommand{
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

// printModAddUsage prints usage for the mod add command.
func printModAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod add --name <name> [--spec <path|->]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --name <name>    Unique name for the mod (required)")
	_, _ = fmt.Fprintln(w, "  --spec <path|->  Path to YAML/JSON spec file (use '-' for stdin)")
}
