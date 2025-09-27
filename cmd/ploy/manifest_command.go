package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// handleManifest routes manifest subcommands.
func handleManifest(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printManifestUsage(stderr)
		return errors.New("manifest subcommand required")
	}

	switch args[0] {
	case "schema":
		return handleManifestSchema(args[1:], stderr)
	default:
		printManifestUsage(stderr)
		return fmt.Errorf("unknown manifest subcommand %q", args[0])
	}
}

// printManifestUsage prints the manifest command usage information.
func printManifestUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy manifest <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  schema  Print the integration manifest JSON schema")
}

// handleManifestSchema writes the manifest schema file to the provided writer.
func handleManifestSchema(args []string, stderr io.Writer) error {
	if len(args) > 0 {
		printManifestSchemaUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
	}

	data, err := os.ReadFile(manifestSchemaPath)
	if err != nil {
		return fmt.Errorf("read manifest schema: %w", err)
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
