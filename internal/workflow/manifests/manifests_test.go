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
manifest_version = "v2"
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

[[services]]
name = "mods-postgres"
kind = "postgres"
[services.identity]
dns = "mods-postgres.svc.local"
[[services.ports]]
name = "psql"
port = 5432
protocol = "tcp"

[[services]]
name = "mods-api"
kind = "http"
[services.identity]
dns = "mods-api.svc.local"
[[services.ports]]
name = "metrics"
port = 9100
protocol = "tcp"
[[services.ports]]
name = "http"
port = 8080
protocol = "tcp"
[[services.requires]]
target = "mods-postgres"
edge = "api->postgres"
[[services.requires]]
target = "mods-redis"
edge = "api->redis"

[[services]]
name = "mods-redis"
kind = "redis"
optional = true
[services.identity]
dns = "mods-redis.svc.local"
[[services.ports]]
name = "tcp"
port = 6379
protocol = "tcp"

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

[[edges]]
source = "mods-api"
target = "mods-redis"
ports = ["tcp"]
protocols = ["tcp"]

[[edges]]
source = "mods-api"
target = "mods-postgres"
ports = ["psql"]
protocols = ["tcp"]

[[exposures]]
service = "mods-api"
port = "http"
mode = "public"

[[exposures]]
service = "mods-api"
port = "metrics"
mode = "cluster"
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
	if decoded["manifest_version"] != "v2" {
		t.Fatalf("expected manifest_version v2, got %+v", decoded["manifest_version"])
	}
	services, ok := decoded["services"].([]any)
	if !ok || len(services) != 3 {
		t.Fatalf("expected 3 services, got %+v", decoded["services"])
	}
	firstService, _ := services[0].(map[string]any)
	if firstService["name"] != "mods-api" {
		t.Fatalf("expected services sorted by name, got %+v", services)
	}
	edges, ok := decoded["edges"].([]any)
	if !ok || len(edges) != 2 {
		t.Fatalf("expected edges array, got %+v", decoded["edges"])
	}
	firstEdge, _ := edges[0].(map[string]any)
	if firstEdge["target"] != "mods-postgres" {
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
	if !strings.Contains(err.Error(), "manifest_version") {
		t.Fatalf("expected manifest_version validation error, got %v", err)
	}
}

func TestRegistryCompileValidatesVersionMatch(t *testing.T) {
	dir := t.TempDir()
	manifest := `
manifest_version = "v2"
name = "smoke"
version = "2025-09-26"
summary = "ok"
[topology]
[[topology.allow]]
from = "a"
to = "b"

[[services]]
name = "svc-a"
kind = "http"
[services.identity]
dns = "svc-a.local"
[[services.ports]]
name = "http"
port = 8080
protocol = "tcp"

[[edges]]
source = "svc-a"
target = "svc-a"
ports = ["http"]
protocols = ["tcp"]

[[exposures]]
service = "svc-a"
port = "http"
mode = "cluster"
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

func TestRegistryCompileRejectsManifestWithoutServices(t *testing.T) {
	dir := t.TempDir()
	manifest := `
manifest_version = "v2"
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
`
	if err := os.WriteFile(filepath.Join(dir, "smoke.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifests.LoadDirectory(dir)
	if err == nil {
		t.Fatal("expected error when services block missing")
	}
	if !strings.Contains(err.Error(), "services") {
		t.Fatalf("expected services validation error, got %v", err)
	}
}

func TestEncodeCompilationToTOMLProducesDeterministicOutput(t *testing.T) {
	dir := t.TempDir()
	manifest := `
manifest_version = "v2"
name = "smoke"
version = "2025-09-26"
summary = "ok"

[topology]
description = "Standard mods topology"
[[topology.allow]]
from = "mods-api"
to = "mods-postgres"

[[services]]
name = "mods-postgres"
kind = "postgres"
[services.identity]
dns = "mods-postgres.svc.local"
[[services.ports]]
name = "psql"
port = 5432
protocol = "tcp"

[[services]]
name = "mods-api"
kind = "http"
[services.identity]
dns = "mods-api.svc.local"
[[services.ports]]
name = "http"
port = 8080
protocol = "tcp"
[[services.requires]]
target = "mods-postgres"
edge = "api->postgres"

[fixtures]
[[fixtures.required]]
name = "postgres"
reference = "snapshot:dev-db"

[lanes]
[[lanes.required]]
name = "go-native"
reason = "baseline"

[aster]
required = ["plan"]
optional = []

[[edges]]
source = "mods-api"
target = "mods-postgres"
ports = ["psql"]
protocols = ["tcp"]
`

	path := filepath.Join(dir, "smoke.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	compiled, err := manifests.LoadFile(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	encoded, err := manifests.EncodeCompilationToTOML(compiled)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	output := string(encoded)
	if !strings.Contains(output, "manifest_version = 'v2'") {
		t.Fatalf("expected manifest_version in encoded output, got %q", output)
	}
	if strings.Count(output, "[[services]]") != 2 {
		t.Fatalf("expected two services in encoded output, got %q", output)
	}
}
