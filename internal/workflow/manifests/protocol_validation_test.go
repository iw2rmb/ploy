package manifests_test

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// TestRegistryLoadRejectsInvalidProtocol ensures manifests using unknown protocols fail validation.
func TestRegistryLoadRejectsInvalidProtocol(t *testing.T) {
	dir := t.TempDir()
	// Use minimal valid manifest but set an invalid protocol value.
	body := `
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
protocol = "http" # invalid
[[services.requires]]
target = "svc-a"
edge = "loop"

[[edges]]
source = "svc-a"
target = "svc-a"
ports = ["http"]
protocols = ["http"] # invalid

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

	writeManifest(t, dir, "smoke.toml", body)
	_, err := manifests.ExportLoadDirectory(dir)
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
	// Ensure error surfaces protocol context.
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("expected protocol validation error, got %v", err)
	}
}
