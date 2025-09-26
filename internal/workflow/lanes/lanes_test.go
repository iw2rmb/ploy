package lanes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirectoryParsesLaneSpecs(t *testing.T) {
	tmp := t.TempDir()
	lanePath := filepath.Join(tmp, "node-wasm.toml")
	content := `name = "node-wasm"

description = "Node.js lane on WASM runtime"
runtime_family = "wasm-node"
cache_namespace = "node-wasm"

[commands]
build = ["npm", "ci"]
test = ["npm", "test"]
`
	if err := os.WriteFile(lanePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write lane file: %v", err)
	}

	reg, err := LoadDirectory(tmp)
	if err != nil {
		t.Fatalf("LoadDirectory returned error: %v", err)
	}
	desc, err := reg.Describe("node-wasm", DescribeOptions{CommitSHA: "abc123"})
	if err != nil {
		t.Fatalf("Describe returned error: %v", err)
	}
	if desc.Lane.Name != "node-wasm" {
		t.Fatalf("unexpected lane name: %s", desc.Lane.Name)
	}
	if desc.Lane.RuntimeFamily != "wasm-node" {
		t.Fatalf("unexpected runtime family: %s", desc.Lane.RuntimeFamily)
	}
	if len(desc.Lane.Commands.Build) == 0 || desc.Lane.Commands.Build[0] != "npm" {
		t.Fatalf("unexpected build commands: %#v", desc.Lane.Commands.Build)
	}
}

func TestLoadDirectoryValidatesRequiredFields(t *testing.T) {
	tmp := t.TempDir()
	lanePath := filepath.Join(tmp, "broken.toml")
	content := `name = "broken"

[commands]
build = ["true"]
`
	if err := os.WriteFile(lanePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write lane file: %v", err)
	}

	_, err := LoadDirectory(tmp)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestLoadDirectoryRequiresDescription(t *testing.T) {
	tmp := t.TempDir()
	lanePath := filepath.Join(tmp, "missing-desc.toml")
	content := `name = "missing-desc"

runtime_family = "custom"
cache_namespace = "missing-desc"

[commands]
build = ["true"]
test = ["true"]
`
	if err := os.WriteFile(lanePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write lane file: %v", err)
	}

	_, err := LoadDirectory(tmp)
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}
