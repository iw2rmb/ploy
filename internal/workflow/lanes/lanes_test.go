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

[job]
image = "registry.dev/node:20"
command = ["npm", "test"]

  [job.env]
  NODE_ENV = "test"

  [job.resources]
  cpu = "2000m"
  memory = "4Gi"
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
	if desc.Lane.Job.Image != "registry.dev/node:20" {
		t.Fatalf("unexpected job image: %s", desc.Lane.Job.Image)
	}
	if len(desc.Lane.Job.Command) != 2 || desc.Lane.Job.Command[0] != "npm" {
		t.Fatalf("unexpected job command: %#v", desc.Lane.Job.Command)
	}
	if got := desc.Lane.Job.Env["NODE_ENV"]; got != "test" {
		t.Fatalf("unexpected job env: %s", got)
	}
	if desc.Lane.Job.Resources.CPU != "2000m" {
		t.Fatalf("unexpected job cpu: %s", desc.Lane.Job.Resources.CPU)
	}
}

func TestLoadDirectoryValidatesRequiredFields(t *testing.T) {
	tmp := t.TempDir()
	lanePath := filepath.Join(tmp, "broken.toml")
	content := `name = "broken"

[commands]
build = ["true"]
test = ["true"]
`
	if err := os.WriteFile(lanePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write lane file: %v", err)
	}

	_, err := LoadDirectory(tmp)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestLoadDirectoryRequiresJobSpec(t *testing.T) {
	tmp := t.TempDir()
	lanePath := filepath.Join(tmp, "missing-job.toml")
	content := `name = "missing-job"

description = "lane missing job spec"
runtime_family = "custom"
cache_namespace = "missing-job"

[commands]
build = ["true"]
test = ["true"]
`
	if err := os.WriteFile(lanePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write lane file: %v", err)
	}

	_, err := LoadDirectory(tmp)
	if err == nil {
		t.Fatal("expected error for missing job spec")
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
