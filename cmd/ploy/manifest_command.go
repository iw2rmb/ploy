package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	manifestcli "github.com/iw2rmb/ploy/internal/cli/manifest"
)

const manifestSchemaPath = "docs/schemas/integration_manifest.schema.json"

// handleManifest routes manifest subcommands.
func handleManifest(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printManifestUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printManifestUsage(stderr)
		return errors.New("manifest subcommand required")
	}

	switch args[0] {
	case "schema":
		return handleManifestSchema(args[1:], stderr)
	case "validate":
		return handleManifestValidate(args[1:], stderr)
	default:
		printManifestUsage(stderr)
		return fmt.Errorf("unknown manifest subcommand %q", args[0])
	}
}

// printManifestUsage prints the manifest command usage information.
func printManifestUsage(w io.Writer) {
	printCommandUsage(w, "manifest")
}

// handleManifestSchema writes the manifest schema file to the provided writer.
func handleManifestSchema(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printManifestSchemaUsage(stderr)
		return nil
	}
	if len(args) > 0 {
		printManifestSchemaUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
	}

	data, err := manifestcli.LoadSchema(manifestSchemaPath)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Ploy integration manifest schema (%s):\n", manifestSchemaPath)
	if _, err := stderr.Write(data); err != nil {
		return fmt.Errorf("write manifest schema: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(stderr)
	}
	return nil
}

// printManifestSchemaUsage displays the schema command usage.
func printManifestSchemaUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy manifest schema")
}

// handleManifestValidate validates manifests and optionally rewrites them in place.
func handleManifestValidate(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printManifestValidateUsage(stderr)
		return nil
	}
	rewrite, targets, err := manifestcli.ParseTargets(args)
	if err != nil {
		printManifestValidateUsage(stderr)
		return err
	}

	results, err := manifestcli.Validate(manifestcli.ValidateOptions{Targets: targets, Rewrite: rewrite})
	if err != nil {
		if errors.Is(err, manifestcli.ErrManifestPathRequired) {
			printManifestValidateUsage(stderr)
		}
		return err
	}

	for _, res := range results {
		if res.Rewritten {
			_, _ = fmt.Fprintf(stderr, "Rewrote manifest %s to v2 (%s@%s)\n", res.Path, res.Name, res.Version)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "Validated manifest %s (%s@%s)\n", res.Path, res.Name, res.Version)
	}
	return nil
}

// printManifestValidateUsage displays usage guidance for the validate command.
func printManifestValidateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy manifest validate [--rewrite=v2] <path> [<path>...]")
}
