package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildSpecPayloadFromYAML verifies that buildSpecPayload correctly parses
// a YAML spec file and produces the expected JSON payload structure.
func TestBuildSpecPayloadFromYAML(t *testing.T) {
	// Create a temporary YAML spec file
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
mod:
  image: docker.io/test/mod:latest
  env:
    KEY1: value1
    KEY2: value2
  retain_container: true
build_gate_healing:
  retries: 1
  mods:
    - image: docker.io/test/healer:latest
gitlab_domain: gitlab.example.com
mr_on_success: true
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify the spec contains build_gate_healing
	if _, ok := result["build_gate_healing"]; !ok {
		t.Errorf("expected build_gate_healing in payload")
	}

	// Verify mod settings
	if mod, ok := result["mod"].(map[string]any); ok {
		if img, ok := mod["image"].(string); !ok || img != "docker.io/test/mod:latest" {
			t.Errorf("expected mod.image=docker.io/test/mod:latest, got %v", mod["image"])
		}
	} else {
		t.Errorf("expected mod in payload")
	}

	// Verify gitlab_domain
	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
		t.Errorf("expected gitlab_domain=gitlab.example.com, got %v", result["gitlab_domain"])
	}

	// Verify mr_on_success
	if success, ok := result["mr_on_success"].(bool); !ok || !success {
		t.Errorf("expected mr_on_success=true, got %v", result["mr_on_success"])
	}
}

// TestBuildSpecPayloadFromJSON verifies that buildSpecPayload correctly parses
// a JSON spec file and produces the expected payload structure.
func TestBuildSpecPayloadFromJSON(t *testing.T) {
	// Create a temporary JSON spec file
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.json")
	specContent := `{
  "image": "docker.io/test/mod:latest",
  "env": {
    "KEY1": "value1"
  },
  "build_gate_healing": {
    "retries": 2,
    "mods": []
  }
}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify the spec contains build_gate_healing
	if healing, ok := result["build_gate_healing"].(map[string]any); ok {
		if retries, ok := healing["retries"].(float64); !ok || retries != 2 {
			t.Errorf("expected build_gate_healing.retries=2, got %v", healing["retries"])
		}
	} else {
		t.Errorf("expected build_gate_healing in payload")
	}
}

// TestBuildSpecPayloadCommand_MergesWithoutCLI verifies that when a spec defines
// mod.command and no --mod-command CLI flag is passed, the spec command is preserved.
func TestBuildSpecPayloadCommand_MergesWithoutCLI(t *testing.T) {
	// Spec defines mod.command; CLI does not pass --mod-command.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := `
mod:
  image: docker.io/test/mod:latest
  command: ["/bin/sh", "-lc", "echo hi"]
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	mod, ok := out["mod"].(map[string]any)
	if !ok {
		t.Fatalf("expected mod section")
	}
	cmd, ok := mod["command"].([]any)
	if !ok || len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-lc" || cmd[2] != "echo hi" {
		t.Fatalf("expected command array preserved, got %v", mod["command"])
	}
}

// TestBuildSpecPayloadCommand_CLIOverridesJSON verifies that a CLI-provided
// JSON array command overrides the spec-defined command.
func TestBuildSpecPayloadCommand_CLIOverridesJSON(t *testing.T) {
	// Spec defines command; CLI provides JSON array to override.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := `
mod:
  command: ["echo", "spec"]
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	payload, err := buildSpecPayload(specPath, nil, "", false, `["echo","cli"]`, "", "", false, false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	mod, ok := out["mod"].(map[string]any)
	if !ok {
		t.Fatalf("expected mod section")
	}
	cmd, ok := mod["command"].([]any)
	if !ok || len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "cli" {
		t.Fatalf("expected command overridden to [echo cli], got %v", mod["command"])
	}
}

// TestBuildSpecPayloadInvalidFile verifies that buildSpecPayload returns an error
// when attempting to read a non-existent spec file.
func TestBuildSpecPayloadInvalidFile(t *testing.T) {
	// Non-existent file should error
	_, err := buildSpecPayload("/nonexistent/path/spec.yaml", nil, "", false, "", "", "", false, false, false)
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

// TestBuildSpecPayloadInvalidFormat verifies that buildSpecPayload returns an error
// when the spec file contains invalid YAML or JSON syntax.
func TestBuildSpecPayloadInvalidFormat(t *testing.T) {
	// Create a file with invalid YAML/JSON
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(specPath, []byte("not: valid: yaml: content:"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false, false)
	if err == nil {
		t.Errorf("expected error for invalid YAML/JSON")
	}
}

// TestBuildSpecPayloadCommandJSONArray verifies that a CLI-provided command
// as a JSON array is correctly parsed and stored in the payload.
func TestBuildSpecPayloadCommandJSONArray(t *testing.T) {
	// Test command as JSON array
	payload, err := buildSpecPayload(
		"",
		nil,
		"",
		false,
		`["/bin/sh", "-c", "echo test"]`,
		"",
		"",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if cmd, ok := result["command"].([]any); ok {
		if len(cmd) != 3 {
			t.Errorf("expected command array length 3, got %d", len(cmd))
		}
		if cmd[0] != "/bin/sh" {
			t.Errorf("expected command[0]=/bin/sh, got %v", cmd[0])
		}
	} else {
		t.Errorf("expected command as array, got %T", result["command"])
	}
}

// TestBuildSpecPayloadCommandString verifies that a CLI-provided command
// as a plain string is correctly stored in the payload.
func TestBuildSpecPayloadCommandString(t *testing.T) {
	// Test command as plain string
	payload, err := buildSpecPayload(
		"",
		nil,
		"",
		false,
		"echo test",
		"",
		"",
		false,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if cmd, ok := result["command"].(string); !ok || cmd != "echo test" {
		t.Errorf("expected command string 'echo test', got %v", result["command"])
	}
}

// TestBuildSpecPayloadContainsBuildGateHealing verifies that complex nested
// structures (like build_gate_healing with retries, mods array) are correctly
// parsed from YAML and preserved in the payload.
func TestBuildSpecPayloadContainsBuildGateHealing(t *testing.T) {
	// Test that build_gate_healing is preserved when present in spec
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
build_gate_healing:
  retries: 2
  mods:
    - image: docker.io/test/healer:latest
      command: "heal.sh"
      env:
        HEALING_MODE: auto
      retain_container: false
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify build_gate_healing is present
	healing, ok := result["build_gate_healing"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate_healing in payload")
	}

	// Verify retries
	if retries, ok := healing["retries"].(float64); !ok || retries != 2 {
		t.Errorf("expected build_gate_healing.retries=2, got %v", healing["retries"])
	}

	// Verify mods array
	mods, ok := healing["mods"].([]any)
	if !ok || len(mods) != 1 {
		t.Fatalf("expected build_gate_healing.mods array with 1 element, got %v", healing["mods"])
	}

	// Verify first mod entry
	mod0, ok := mods[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first mod to be a map, got %T", mods[0])
	}

	if img, ok := mod0["image"].(string); !ok || img != "docker.io/test/healer:latest" {
		t.Errorf("expected first mod.image=docker.io/test/healer:latest, got %v", mod0["image"])
	}
}
