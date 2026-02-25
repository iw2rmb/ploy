package manifests_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

func TestLoadFile_DecodeErrorIncludesPath(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, "broken.toml", `manifest_version = "v2"`+"\n"+`name =`)

	_, err := manifests.LoadFile(path)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode manifest") {
		t.Fatalf("expected decode error prefix, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected error to include path %q, got %v", path, err)
	}
}

func TestLoadFile_ValidationErrorIncludesPath(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, "invalid.toml", `
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

	_, err := manifests.LoadFile(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid manifest configuration") {
		t.Fatalf("expected invalid manifest error, got %v", err)
	}
	if !strings.Contains(err.Error(), "("+path+")") {
		t.Fatalf("expected path in validation error, got %v", err)
	}
}

func TestLoadDirectory_DecodeErrorIncludesFileName(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "broken.toml", `manifest_version = "v2"`+"\n"+`name =`)

	_, err := manifests.ExportLoadDirectory(dir)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode manifest broken.toml") {
		t.Fatalf("expected filename in decode error, got %v", err)
	}
	if strings.Contains(err.Error(), filepath.Join(dir, "broken.toml")) {
		t.Fatalf("expected directory loader to report filename only, got %v", err)
	}
}

