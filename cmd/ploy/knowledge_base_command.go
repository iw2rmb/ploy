package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
)

// handleKnowledgeBase routes knowledge-base subcommands.
func handleKnowledgeBase(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printKnowledgeBaseUsage(stderr)
		return errors.New("knowledge-base subcommand required")
	}

	switch args[0] {
	case "ingest":
		return handleKnowledgeBaseIngest(args[1:], stderr)
	default:
		printKnowledgeBaseUsage(stderr)
		return fmt.Errorf("unknown knowledge-base subcommand %q", args[0])
	}
}

func handleKnowledgeBaseIngest(args []string, stderr io.Writer) error {
	flagSet := flag.NewFlagSet("knowledge-base ingest", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	from := flagSet.String("from", "", "path to knowledge base incident fixture (JSON)")
	if err := flagSet.Parse(args); err != nil {
		printKnowledgeBaseIngestUsage(stderr)
		return err
	}
	fixturePath := strings.TrimSpace(*from)
	if fixturePath == "" {
		printKnowledgeBaseIngestUsage(stderr)
		return fmt.Errorf("fixture path required")
	}

	fixture, err := knowledgebase.LoadCatalogFile(fixturePath)
	if err != nil {
		printKnowledgeBaseIngestUsage(stderr)
		return fmt.Errorf("load fixture: %w", err)
	}

	current, err := knowledgebase.LoadCatalogFile(knowledgeBaseCatalogPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, os.ErrNotExist) {
			printKnowledgeBaseIngestUsage(stderr)
			return fmt.Errorf("load catalog: %w", err)
		}
		current = knowledgebase.Catalog{}
	}

	merged, err := knowledgebase.MergeCatalog(current, fixture)
	if err != nil {
		printKnowledgeBaseIngestUsage(stderr)
		return err
	}

	if err := knowledgebase.SaveCatalogFile(knowledgeBaseCatalogPath, merged); err != nil {
		printKnowledgeBaseIngestUsage(stderr)
		return fmt.Errorf("save catalog: %w", err)
	}

	ids := make([]string, len(fixture.Incidents))
	for i, incident := range fixture.Incidents {
		ids[i] = incident.ID
	}
	if len(ids) == 0 {
		_, _ = fmt.Fprintln(stderr, "No incidents supplied; catalog unchanged")
		return nil
	}
	_, _ = fmt.Fprintf(stderr, "Ingested %d incident(s): %s\n", len(ids), strings.Join(ids, ", "))
	return nil
}

func printKnowledgeBaseUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy knowledge-base <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  ingest    Append incidents to the knowledge base catalog")
}

func printKnowledgeBaseIngestUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy knowledge-base ingest --from <fixture.json>")
}
