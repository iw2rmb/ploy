package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestHandleKnowledgeBaseIngestAppendsIncidents(t *testing.T) {
	dir := t.TempDir()
	prevCatalog := knowledgeBaseCatalogPath
	knowledgeBaseCatalogPath = filepath.Join(dir, "catalog.json")
	t.Cleanup(func() { knowledgeBaseCatalogPath = prevCatalog })

	initialCatalog := map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{
				"id":         "existing-incident",
				"errors":     []string{"existing failure"},
				"recipes":    []string{"recipe.old"},
				"summary":    "Existing summary",
				"human_gate": false,
			},
		},
	}
	writeJSON(t, knowledgeBaseCatalogPath, initialCatalog)

	fixturePath := filepath.Join(dir, "fixture.json")
	fixture := map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{
				"id":         "new-incident",
				"errors":     []string{"npm ERR! missing script"},
				"recipes":    []string{"recipe.npm.fix"},
				"summary":    "Add npm start script",
				"human_gate": true,
			},
		},
	}
	writeJSON(t, fixturePath, fixture)

	stderr := &bytes.Buffer{}
	err := handleKnowledgeBase([]string{"ingest", "--from", fixturePath}, stderr)
	if err != nil {
		t.Fatalf("expected ingest to succeed, got %v (stderr: %s)", err, stderr.String())
	}

	catalogBytes, err := os.ReadFile(knowledgeBaseCatalogPath)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		SchemaVersion string `json:"schema_version"`
		Incidents     []struct {
			ID string `json:"id"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if catalog.SchemaVersion != "2025-09-27.1" {
		t.Fatalf("expected schema version preserved, got %s", catalog.SchemaVersion)
	}
	ids := make([]string, len(catalog.Incidents))
	for i, incident := range catalog.Incidents {
		ids[i] = incident.ID
	}
	expect := []string{"existing-incident", "new-incident"}
	if !equalStringSlices(ids, expect) {
		t.Fatalf("expected incidents %v, got %v", expect, ids)
	}
	if !strings.Contains(stderr.String(), "new-incident") {
		t.Fatalf("expected stderr summary to reference ingested incident, got %q", stderr.String())
	}
}

func TestHandleKnowledgeBaseIngestDetectsDuplicate(t *testing.T) {
	dir := t.TempDir()
	prevCatalog := knowledgeBaseCatalogPath
	knowledgeBaseCatalogPath = filepath.Join(dir, "catalog.json")
	t.Cleanup(func() { knowledgeBaseCatalogPath = prevCatalog })

	initial := map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{"id": "duplicate", "errors": []string{"existing"}, "recipes": []string{"recipe.old"}, "summary": "Existing", "human_gate": false},
		},
	}
	writeJSON(t, knowledgeBaseCatalogPath, initial)

	fixturePath := filepath.Join(dir, "dup.json")
	fixture := map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{"id": "duplicate", "errors": []string{"duplicate"}, "recipes": []string{"recipe.new"}, "summary": "Duplicate", "human_gate": true},
		},
	}
	writeJSON(t, fixturePath, fixture)

	err := handleKnowledgeBase([]string{"ingest", "--from", fixturePath}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected duplicate ingest to fail")
	}
	catalogData := readJSON(t, knowledgeBaseCatalogPath)
	incidents := catalogData["incidents"].([]any)
	if len(incidents) != 1 {
		t.Fatalf("expected catalog unchanged on duplicate, got %d incidents", len(incidents))
	}
}

func TestHandleKnowledgeBaseEvaluateReportsAccuracy(t *testing.T) {
	dir := t.TempDir()
	prevCatalog := knowledgeBaseCatalogPath
	knowledgeBaseCatalogPath = filepath.Join(dir, "catalog.json")
	t.Cleanup(func() { knowledgeBaseCatalogPath = prevCatalog })

	writeJSON(t, knowledgeBaseCatalogPath, map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{
				"id":         "lint-failure",
				"errors":     []string{"npm ERR! lint script failed"},
				"recipes":    []string{"recipe.npm.lint"},
				"summary":    "Run npm run lint",
				"human_gate": true,
			},
		},
	})

	fixturePath := filepath.Join(dir, "evaluate.json")
	writeJSON(t, fixturePath, map[string]any{
		"schema_version": "2025-09-27.1",
		"samples": []map[string]any{
			{
				"name":     "lint-sample",
				"errors":   []string{"npm ERR! lint script failed"},
				"expected": "lint-failure",
			},
			{
				"name":     "unknown",
				"errors":   []string{"completely unknown error"},
				"expected": "lint-failure",
			},
		},
	})

	stderr := &bytes.Buffer{}
	if err := handleKnowledgeBase([]string{"evaluate", "--fixture", fixturePath}, stderr); err != nil {
		t.Fatalf("expected evaluate to succeed, got %v (stderr: %s)", err, stderr.String())
	}
	output := stderr.String()
	if !strings.Contains(output, "lint-sample: expected lint-failure, matched lint-failure (score") || !strings.Contains(output, "[PASS]") {
		t.Fatalf("expected lint sample result in output, got %q", output)
	}
	if !strings.Contains(output, "unknown: expected lint-failure, no match [MISS]") {
		t.Fatalf("expected unknown sample miss in output, got %q", output)
	}
	if !strings.Contains(output, "Summary: matches=1 misses=1 accuracy=50.00%") {
		t.Fatalf("expected summary metrics in output, got %q", output)
	}
}

func TestHandleKnowledgeBaseEvaluateMissingFixture(t *testing.T) {
	stderr := &bytes.Buffer{}
	err := handleKnowledgeBase([]string{"evaluate", "--fixture", ""}, stderr)
	if err == nil {
		t.Fatalf("expected evaluate to fail when fixture missing")
	}
	if !strings.Contains(stderr.String(), "Usage: ploy knowledge-base evaluate") {
		t.Fatalf("expected evaluate usage in stderr, got %q", stderr.String())
	}
}

func TestHandleKnowledgeBaseEvaluateMissingCatalog(t *testing.T) {
	dir := t.TempDir()
	prevCatalog := knowledgeBaseCatalogPath
	knowledgeBaseCatalogPath = filepath.Join(dir, "missing.json")
	t.Cleanup(func() { knowledgeBaseCatalogPath = prevCatalog })

	fixturePath := filepath.Join(dir, "evaluate.json")
	writeJSON(t, fixturePath, map[string]any{
		"schema_version": "2025-09-27.1",
		"samples": []map[string]any{
			{"name": "noop", "errors": []string{"err"}, "expected": "lint-failure"},
		},
	})

	stderr := &bytes.Buffer{}
	err := handleKnowledgeBase([]string{"evaluate", "--fixture", fixturePath}, stderr)
	if err == nil {
		t.Fatalf("expected evaluate to fail when catalog missing")
	}
	if !strings.Contains(stderr.String(), "Usage: ploy knowledge-base evaluate") {
		t.Fatalf("expected evaluate usage when catalog missing, got %q", stderr.String())
	}
}

func TestHandleKnowledgeBaseUsage(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleKnowledgeBase(nil, buf)
	if err == nil {
		t.Fatalf("expected error when subcommand missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy knowledge-base") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestExecuteRoutesKnowledgeBase(t *testing.T) {
	dir := t.TempDir()
	prevCatalog := knowledgeBaseCatalogPath
	knowledgeBaseCatalogPath = filepath.Join(dir, "catalog.json")
	t.Cleanup(func() { knowledgeBaseCatalogPath = prevCatalog })
	writeJSON(t, knowledgeBaseCatalogPath, map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents":      []map[string]any{},
	})

	fixturePath := filepath.Join(dir, "fixture.json")
	writeJSON(t, fixturePath, map[string]any{
		"schema_version": "2025-09-27.1",
		"incidents": []map[string]any{
			{"id": "top-level", "errors": []string{"err"}, "recipes": []string{"recipe"}, "summary": "Top", "human_gate": false},
		},
	})

	stderr := &bytes.Buffer{}
	err := execute([]string{"knowledge-base", "ingest", "--from", fixturePath}, stderr)
	if err != nil {
		t.Fatalf("expected top-level ingest to succeed, got %v", err)
	}
}

func writeJSON(t *testing.T, path string, payload any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	return raw
}

func equalStringSlices(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	if !sort.StringsAreSorted(actual) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}
