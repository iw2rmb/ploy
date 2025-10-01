package manifests_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSchemaAssetIsWellFormed(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	schemaPath := filepath.Join(repoRoot, "docs", "schemas", "integration_manifest.schema.json")

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema json: %v", err)
	}

	if schema["title"] != "Ploy Integration Manifest" {
		t.Fatalf("unexpected schema title: %v", schema["title"])
	}

	if schema["type"] != "object" {
		t.Fatalf("unexpected schema type: %v", schema["type"])
	}

	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required field missing or invalid: %v", schema["required"])
	}

	want := map[string]struct{}{
		"manifest_version": {},
		"name":             {},
		"version":          {},
		"summary":          {},
		"topology":         {},
		"fixtures":         {},
		"lanes":            {},
		"services":         {},
		"edges":            {},
	}

	for _, item := range required {
		if key, ok := item.(string); ok {
			delete(want, key)
		}
	}

	if len(want) != 0 {
		t.Fatalf("schema missing required keys: %v", want)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties field missing")
	}
	if _, ok := properties["services"].(map[string]any); !ok {
		t.Fatalf("services property missing: %v", properties["services"])
	}
	if _, ok := properties["edges"].(map[string]any); !ok {
		t.Fatalf("edges property missing: %v", properties["edges"])
	}
	if _, ok := properties["exposures"].(map[string]any); !ok {
		t.Fatalf("exposures property missing: %v", properties["exposures"])
	}
	fixtures, ok := properties["fixtures"].(map[string]any)
	if !ok {
		t.Fatalf("fixtures property missing: %v", properties["fixtures"])
	}
	fixturesRequired, ok := fixtures["required"].([]any)
	if !ok {
		t.Fatalf("fixtures.required constraint missing: %v", fixtures["required"])
	}
	seen := map[string]struct{}{}
	for _, item := range fixturesRequired {
		if key, ok := item.(string); ok {
			seen[key] = struct{}{}
		}
	}
	if _, ok := seen["required"]; !ok {
		t.Fatalf("fixtures.required must be required: %v", fixturesRequired)
	}
}
