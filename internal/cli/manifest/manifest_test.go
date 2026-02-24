package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleManifest = `
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

func TestValidateCollectsFiles(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.toml")
	if err := os.WriteFile(manifestPath, []byte(sampleManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	results, err := Validate(ValidateOptions{Targets: []string{dir}})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Path != manifestPath {
		t.Fatalf("expected path %s, got %s", manifestPath, results[0].Path)
	}
	if results[0].Rewritten {
		t.Fatalf("expected rewritten=false")
	}
}

func TestValidateRewrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rewrite.toml")
	if err := os.WriteFile(target, []byte(sampleManifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	results, err := Validate(ValidateOptions{Targets: []string{target}, Rewrite: true})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(results) != 1 || !results[0].Rewritten {
		t.Fatalf("expected rewritten result")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read rewritten manifest: %v", err)
	}
	if string(after) == string(before) {
		t.Fatalf("expected manifest to change after rewrite")
	}
}

func TestParseTargetsFlags(t *testing.T) {
	rewrite, targets, err := ParseTargets([]string{"--rewrite=v2", "path/to/file"})
	if err != nil {
		t.Fatalf("ParseTargets error: %v", err)
	}
	if !rewrite {
		t.Fatalf("expected rewrite flag")
	}
	if len(targets) != 1 || targets[0] != "path/to/file" {
		t.Fatalf("unexpected targets %v", targets)
	}
}

func TestParseTargetsErrorCases(t *testing.T) {
	if _, _, err := ParseTargets(nil); err == nil {
		t.Fatalf("expected error for missing targets")
	}
	if _, _, err := ParseTargets([]string{"--unknown"}); err == nil {
		t.Fatalf("expected error for unknown flag")
	}
}
