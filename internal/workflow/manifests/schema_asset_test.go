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
		"name":     {},
		"version":  {},
		"summary":  {},
		"topology": {},
		"fixtures": {},
		"lanes":    {},
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
