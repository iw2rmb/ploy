package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	plan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

// Default catalog path for knowledge-base commands.
var knowledgeBaseCatalogPath = "configs/knowledge-base/catalog.json"

// handleKnowledgeBase routes knowledge-base subcommands.
func handleKnowledgeBase(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printKnowledgeBaseUsage(stderr)
		return errors.New("knowledge-base subcommand required")
	}

	switch args[0] {
	case "ingest":
		return handleKnowledgeBaseIngest(args[1:], stderr)
	case "evaluate":
		return handleKnowledgeBaseEvaluate(args[1:], stderr)
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

// handleKnowledgeBaseEvaluate runs the evaluation command to score classifier accuracy.
func handleKnowledgeBaseEvaluate(args []string, stderr io.Writer) error {
	flagSet := flag.NewFlagSet("knowledge-base evaluate", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	fixture := flagSet.String("fixture", "", "path to evaluation fixture (JSON)")
	if err := flagSet.Parse(args); err != nil {
		printKnowledgeBaseEvaluateUsage(stderr)
		return err
	}

	fixturePath := strings.TrimSpace(*fixture)
	if fixturePath == "" {
		printKnowledgeBaseEvaluateUsage(stderr)
		return fmt.Errorf("fixture path required")
	}

	catalog, err := knowledgebase.LoadCatalogFile(knowledgeBaseCatalogPath)
	if err != nil {
		printKnowledgeBaseEvaluateUsage(stderr)
		return fmt.Errorf("load catalog: %w", err)
	}

	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{Catalog: catalog, ScoreFloor: 0.5})
	if err != nil {
		printKnowledgeBaseEvaluateUsage(stderr)
		return fmt.Errorf("build advisor: %w", err)
	}

	fixtureData, err := loadKnowledgeBaseEvaluateFixture(fixturePath)
	if err != nil {
		printKnowledgeBaseEvaluateUsage(stderr)
		return err
	}

	matches := 0
	misses := 0
	for idx, sample := range fixtureData.Samples {
		name := strings.TrimSpace(sample.Name)
		if name == "" {
			name = fmt.Sprintf("sample-%d", idx+1)
		}
		expected := strings.TrimSpace(sample.Expected)
		if expected == "" {
			printKnowledgeBaseEvaluateUsage(stderr)
			return fmt.Errorf("sample %s missing expected incident id", name)
		}
		errorsList := normalizeEvaluateErrors(sample.Errors)
		if len(errorsList) == 0 {
			printKnowledgeBaseEvaluateUsage(stderr)
			return fmt.Errorf("sample %s requires at least one error expression", name)
		}
		match, ok, err := advisor.Match(context.Background(), plan.AdviceRequest{Signals: plan.AdviceSignals{Errors: errorsList}})
		if err != nil {
			printKnowledgeBaseEvaluateUsage(stderr)
			return fmt.Errorf("evaluate sample %s: %w", name, err)
		}
		if !ok {
			_, _ = fmt.Fprintf(stderr, "%s: expected %s, no match [MISS]\n", name, expected)
			misses++
			continue
		}
		if match.IncidentID == expected {
			_, _ = fmt.Fprintf(stderr, "%s: expected %s, matched %s (score %.2f) [PASS]\n", name, expected, match.IncidentID, match.Score)
			matches++
		} else {
			_, _ = fmt.Fprintf(stderr, "%s: expected %s, matched %s (score %.2f) [MISS]\n", name, expected, match.IncidentID, match.Score)
			misses++
		}
	}

	total := len(fixtureData.Samples)
	accuracy := 0.0
	if total > 0 {
		accuracy = float64(matches) / float64(total) * 100
	}
	_, _ = fmt.Fprintf(stderr, "Summary: matches=%d misses=%d accuracy=%.2f%%\n", matches, misses, accuracy)
	return nil
}

func printKnowledgeBaseUsage(w io.Writer) {
	printCommandUsage(w, "knowledge-base")
}

func printKnowledgeBaseIngestUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy knowledge-base ingest --from <fixture.json>")
}

// printKnowledgeBaseEvaluateUsage emits usage details for the evaluate subcommand.
func printKnowledgeBaseEvaluateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy knowledge-base evaluate --fixture <fixture.json>")
}

// loadKnowledgeBaseEvaluateFixture decodes the evaluation fixture from disk.
func loadKnowledgeBaseEvaluateFixture(path string) (knowledgeBaseEvaluateFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return knowledgeBaseEvaluateFixture{}, fmt.Errorf("load fixture: %w", err)
	}
	var fx knowledgeBaseEvaluateFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		return knowledgeBaseEvaluateFixture{}, fmt.Errorf("decode fixture: %w", err)
	}
	return fx, nil
}

// normalizeEvaluateErrors trims whitespace and filters empty errors from samples.
func normalizeEvaluateErrors(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if entry := strings.TrimSpace(value); entry != "" {
			trimmed = append(trimmed, entry)
		}
	}
	return trimmed
}

type knowledgeBaseEvaluateFixture struct {
	SchemaVersion string                        `json:"schema_version"`
	Samples       []knowledgeBaseEvaluateSample `json:"samples"`
}

type knowledgeBaseEvaluateSample struct {
	Name     string   `json:"name"`
	Errors   []string `json:"errors"`
	Expected string   `json:"expected"`
}
