package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleManifestValidateRequiresPath(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleManifestValidate(nil, buf)
	if err == nil {
		t.Fatal("expected error when no manifest path provided")
	}
	if !strings.Contains(buf.String(), "Usage: ploy manifest validate") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleManifestValidateAcceptsValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `
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
	path := filepath.Join(dir, "smoke.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := handleManifestValidate([]string{path}, buf); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if !strings.Contains(buf.String(), "Validated manifest") {
		t.Fatalf("expected validation message, got %q", buf.String())
	}
}

func TestHandleManifestValidateRewriteUpdatesManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `
manifest_version = "v2"
name = "smoke"
version = "2025-09-26"
summary = "rewrite"

[topology]
description = "Standard migs topology"
[[topology.allow]]
from = "migs-api"
to = "migs-postgres"

[[services]]
name = "migs-redis"
kind = "redis"
optional = true
[services.identity]
dns = "migs-redis.svc.local"
[[services.ports]]
name = "tcp"
port = 6379
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
[[services.requires]]
target = "migs-redis"
edge = "api->redis"

[[services]]
name = "migs-postgres"
kind = "postgres"
[services.identity]
dns = "migs-postgres.svc.local"
[[services.ports]]
name = "psql"
port = 5432
protocol = "tcp"

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

[[edges]]
source = "migs-api"
target = "migs-redis"
ports = ["tcp"]
protocols = ["tcp"]
`
	path := filepath.Join(dir, "smoke.toml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := handleManifestValidate([]string{"--rewrite=v2", path}, buf); err != nil {
		t.Fatalf("unexpected rewrite error: %v", err)
	}
	if !strings.Contains(buf.String(), "Rewrote manifest") {
		t.Fatalf("expected rewrite message, got %q", buf.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten manifest: %v", err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("expected rewrite to change manifest content")
	}
	if !strings.Contains(string(after), "manifest_version = 'v2'") {
		t.Fatalf("expected manifest_version to be retained, got %q", string(after))
	}
}
