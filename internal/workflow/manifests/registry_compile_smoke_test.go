package manifests_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

const smokeManifestFile = "smoke_registry.toml"

// TestRegistryCompileProvidesNormalizedPayload ensures the smoke manifest compiles into normalized structs and JSON payload.
func TestRegistryCompileProvidesNormalizedPayload(t *testing.T) {
	dir := t.TempDir()
	copyManifestFromTestdata(t, dir, smokeManifestFile)

	compiled := compileManifest(t, dir, manifests.ExportCompileOptions{Name: "smoke", Version: "2025-09-26"})

	if compiled.Manifest.Name != "smoke" {
		t.Fatalf("unexpected manifest name: %s", compiled.Manifest.Name)
	}
	if compiled.Manifest.Version != "2025-09-26" {
		t.Fatalf("unexpected manifest version: %s", compiled.Manifest.Version)
	}
	if !strings.Contains(compiled.Manifest.Summary, "Purpose") {
		t.Fatalf("summary not preserved: %s", compiled.Manifest.Summary)
	}

	if len(compiled.Topology.Allow) != 2 {
		t.Fatalf("expected 2 allow flows, got %d", len(compiled.Topology.Allow))
	}
	if compiled.Topology.Allow[0].From != "migs-api" || compiled.Topology.Allow[0].To != "postgres" {
		t.Fatalf("unexpected first flow: %+v", compiled.Topology.Allow[0])
	}
	if len(compiled.Topology.Deny) != 1 || compiled.Topology.Deny[0].Reason == "" {
		t.Fatalf("expected deny flow with reason, got %+v", compiled.Topology.Deny)
	}

	if len(compiled.Fixtures.Required) != 2 {
		t.Fatalf("expected required fixtures, got %v", compiled.Fixtures.Required)
	}
	if len(compiled.Fixtures.Optional) != 1 {
		t.Fatalf("expected optional fixture, got %v", compiled.Fixtures.Optional)
	}
	if len(compiled.Lanes.Required) != 2 {
		t.Fatalf("expected required lanes, got %v", compiled.Lanes.Required)
	}
	if compiled.Lanes.Required[0].Name == "" || compiled.Lanes.Required[0].Reason == "" {
		t.Fatalf("required lane missing fields: %+v", compiled.Lanes.Required[0])
	}

	payload, err := compiled.JSON()
	if err != nil {
		t.Fatalf("json: %v", err)
	}

	decoded := map[string]any{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	manifestJSON, ok := decoded["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("expected manifest object, got %+v", decoded["manifest"])
	}
	if manifestJSON["name"] != "smoke" {
		t.Fatalf("expected manifest name in json, got %+v", manifestJSON)
	}
	if decoded["manifest_version"] != "v2" {
		t.Fatalf("expected manifest_version v2, got %+v", decoded["manifest_version"])
	}

	services, ok := decoded["services"].([]any)
	if !ok || len(services) != 3 {
		t.Fatalf("expected 3 services, got %+v", decoded["services"])
	}
	firstService, _ := services[0].(map[string]any)
	if firstService["name"] != "migs-api" {
		t.Fatalf("expected services sorted by name, got %+v", services)
	}

	edges, ok := decoded["edges"].([]any)
	if !ok || len(edges) != 2 {
		t.Fatalf("expected edges array, got %+v", decoded["edges"])
	}
	firstEdge, _ := edges[0].(map[string]any)
	if firstEdge["target"] != "migs-postgres" {
		t.Fatalf("expected edges sorted by target, got %+v", edges)
	}

	exposures, ok := decoded["exposures"].([]any)
	if !ok || len(exposures) != 2 {
		t.Fatalf("expected exposures array, got %+v", decoded["exposures"])
	}
	firstExposure, _ := exposures[0].(map[string]any)
	if firstExposure["mode"] != "cluster" {
		t.Fatalf("expected exposures sorted to surface cluster first, got %+v", exposures)
	}
}
