package manifests_test

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// TestRegistryCompileRejectsInvalidManifest verifies manifests without required metadata fail to load.
func TestRegistryCompileRejectsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "broken.toml", `version = "2025-09-26"`)

	_, err := manifests.ExportLoadDirectory(dir)
	if err == nil {
		t.Fatal("expected error for missing name and manifest_version")
	}
	if !strings.Contains(err.Error(), "manifest_version") {
		t.Fatalf("expected manifest_version validation error, got %v", err)
	}
}

const minimalManifest = `
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
[[services.requires]]
target = "svc-a"
edge = "loop"

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
`

// TestRegistryCompileValidatesVersionMatch ensures compile rejects mismatched manifest versions.
func TestRegistryCompileValidatesVersionMatch(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "smoke.toml", minimalManifest)

	_, err := manifests.ExportCompileFromDir(dir, manifests.ExportCompileOptions{Name: "smoke", Version: "other"})
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("expected version mismatch error, got %v", err)
	}
}

// TestRegistryCompileRejectsManifestWithoutServices confirms manifests without services fail validation.
func TestRegistryCompileRejectsManifestWithoutServices(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "smoke.toml", `
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
`)

	_, err := manifests.ExportLoadDirectory(dir)
	if err == nil {
		t.Fatal("expected error when services block missing")
	}
	if !strings.Contains(err.Error(), "services") {
		t.Fatalf("expected services validation error, got %v", err)
	}
}
