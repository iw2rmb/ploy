package manifests_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

func TestRegistryCompileProvidesNormalizedPayload(t *testing.T) {
	dir := t.TempDir()
	manifest := `
name = "smoke"
version = "2025-09-26"
summary = """
## Purpose
Ensure mods workflows stick to baseline caches and fixtures.
"""

[topology]
description = "Standard mods topology"

[[topology.allow]]
from = "mods-api"
to = "postgres"

[[topology.allow]]
from = "mods-api"
to = "redis"

[[topology.deny]]
from = "mods-api"
to = "internet"
reason = "Block egress"

[fixtures]

[[fixtures.required]]
name = "postgres"
reference = "snapshot:dev-db"

[[fixtures.required]]
name = "redis"
reference = "cache:redis"

[[fixtures.optional]]
name = "elasticsearch"
reference = "service:es"
reason = "Only when search stack enabled"

[lanes]

[[lanes.required]]
name = "node-wasm"
reason = "mods DAG"

[[lanes.required]]
name = "go-native"
reason = "build/test baseline"

[[lanes.allowed]]
name = "python-slim"
reason = "data tooling"

[aster]
required = ["plan"]
optional = ["exec", "lint"]
`

	path := filepath.Join(dir, "smoke.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	registry, err := manifests.LoadDirectory(dir)
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	compiled, err := registry.Compile(manifests.CompileOptions{Name: "smoke", Version: "2025-09-26"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

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
	if compiled.Topology.Allow[0].From != "mods-api" || compiled.Topology.Allow[0].To != "postgres" {
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
	if len(compiled.Aster.Required) != 1 || len(compiled.Aster.Optional) != 2 {
		t.Fatalf("unexpected aster toggles: %+v", compiled.Aster)
	}
	if compiled.Aster.Optional[0] != "exec" || compiled.Aster.Optional[1] != "lint" {
		t.Fatalf("expected sorted optional toggles, got %+v", compiled.Aster.Optional)
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
}

func TestRegistryCompileRejectsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	bad := `version = "2025-09-26"`
	if err := os.WriteFile(filepath.Join(dir, "broken.toml"), []byte(bad), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifests.LoadDirectory(dir)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected actionable error, got %v", err)
	}
}

func TestRegistryCompileValidatesVersionMatch(t *testing.T) {
	dir := t.TempDir()
	manifest := `
name = "smoke"
version = "2025-09-26"
summary = "ok"
[topology]
[[topology.allow]]
from = "a"
to = "b"
[fixtures]
[[fixtures.required]]
name = "x"
reference = "y"
[lanes]
[[lanes.required]]
name = "go-native"
reason = "build-gate"
[aster]
required = []
optional = []
`
	if err := os.WriteFile(filepath.Join(dir, "smoke.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	registry, err := manifests.LoadDirectory(dir)
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	_, err = registry.Compile(manifests.CompileOptions{Name: "smoke", Version: "other"})
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("expected version mismatch error, got %v", err)
	}
}
