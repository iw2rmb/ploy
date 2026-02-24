package manifests_test

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

const encodeManifest = `
manifest_version = "v2"
name = "smoke"
version = "2025-09-26"
summary = "ok"

[topology]
description = "Standard migs topology"
[[topology.allow]]
from = "migs-api"
to = "migs-postgres"

[[services]]
name = "migs-postgres"
kind = "postgres"
[services.identity]
dns = "migs-postgres.svc.local"
[[services.ports]]
name = "psql"
port = 5432
protocol = "tcp"

[[services]]
name = "migs-api"
kind = "http"
[services.identity]
dns = "migs-api.svc.local"
[[services.ports]]
name = "http"
port = 8080
protocol = "tcp"
[[services.requires]]
target = "migs-postgres"
edge = "api->postgres"

[fixtures]
[[fixtures.required]]
name = "postgres"
reference = "snapshot:dev-db"

[lanes]
[[lanes.required]]
name = "go-native"
reason = "baseline"

[[edges]]
source = "migs-api"
target = "migs-postgres"
ports = ["psql"]
protocols = ["tcp"]
`

// TestEncodeCompilationToTOMLProducesDeterministicOutput verifies EncodeCompilationToTOML sorts fields consistently.
func TestEncodeCompilationToTOMLProducesDeterministicOutput(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, "smoke.toml", encodeManifest)

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
