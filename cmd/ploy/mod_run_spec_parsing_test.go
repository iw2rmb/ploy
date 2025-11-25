package main

import (
	"encoding/json"
	"fmt"
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

// TestBuildSpecPayloadMultiStepMods verifies that the mods[] array is correctly
// parsed and preserved when using multi-step mod format. The mods[] array
// represents sequential transformation steps sharing a global gate/heal policy.
func TestBuildSpecPayloadMultiStepMods(t *testing.T) {
	// Create a spec with multi-step mods[] array
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "multi.yaml")
	specContent := `
apiVersion: ploy.mod/v1alpha1
kind: ModRunSpec
mods:
  - image: docker.io/test/mod-step1:latest
    env:
      STEP: "1"
      TARGET: java8
  - image: docker.io/test/mod-step2:latest
    env:
      STEP: "2"
      TARGET: java11
  - image: docker.io/test/mod-step3:latest
    env:
      STEP: "3"
      TARGET: java17
build_gate:
  enabled: true
  profile: auto
build_gate_healing:
  retries: 1
  mods:
    - image: docker.io/test/healer:latest
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

	// Verify mods[] array exists and has 3 entries
	mods, ok := result["mods"].([]any)
	if !ok {
		t.Fatalf("expected mods array in payload, got %T", result["mods"])
	}
	if len(mods) != 3 {
		t.Fatalf("expected 3 mods in array, got %d", len(mods))
	}

	// Verify first mod step
	mod0, ok := mods[0].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[0] to be map, got %T", mods[0])
	}
	if img, ok := mod0["image"].(string); !ok || img != "docker.io/test/mod-step1:latest" {
		t.Errorf("expected mods[0].image=docker.io/test/mod-step1:latest, got %v", mod0["image"])
	}
	env0, ok := mod0["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[0].env to be map, got %T", mod0["env"])
	}
	if step, ok := env0["STEP"].(string); !ok || step != "1" {
		t.Errorf("expected mods[0].env.STEP=1, got %v", env0["STEP"])
	}

	// Verify second mod step
	mod1, ok := mods[1].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[1] to be map, got %T", mods[1])
	}
	if img, ok := mod1["image"].(string); !ok || img != "docker.io/test/mod-step2:latest" {
		t.Errorf("expected mods[1].image=docker.io/test/mod-step2:latest, got %v", mod1["image"])
	}

	// Verify third mod step
	mod2, ok := mods[2].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[2] to be map, got %T", mods[2])
	}
	if img, ok := mod2["image"].(string); !ok || img != "docker.io/test/mod-step3:latest" {
		t.Errorf("expected mods[2].image=docker.io/test/mod-step3:latest, got %v", mod2["image"])
	}

	// Verify global build_gate and build_gate_healing are preserved
	if _, ok := result["build_gate"].(map[string]any); !ok {
		t.Errorf("expected build_gate in payload")
	}
	if _, ok := result["build_gate_healing"].(map[string]any); !ok {
		t.Errorf("expected build_gate_healing in payload")
	}
}

// TestBuildSpecPayloadMultiStepModsWithEnvFromFile verifies that env_from_file
// resolution works correctly for each mod entry in the mods[] array.
func TestBuildSpecPayloadMultiStepModsWithEnvFromFile(t *testing.T) {
	// Create temp files for env_from_file references
	tmpDir := t.TempDir()
	envFile1 := filepath.Join(tmpDir, "env1.txt")
	envFile2 := filepath.Join(tmpDir, "env2.txt")
	if err := os.WriteFile(envFile1, []byte("secret-token-1"), 0o644); err != nil {
		t.Fatalf("write env file 1: %v", err)
	}
	if err := os.WriteFile(envFile2, []byte("secret-token-2"), 0o644); err != nil {
		t.Fatalf("write env file 2: %v", err)
	}

	// Create spec with mods[] using env_from_file
	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := fmt.Sprintf(`
mods:
  - image: docker.io/test/mod1:latest
    env_from_file:
      TOKEN: %s
  - image: docker.io/test/mod2:latest
    env_from_file:
      TOKEN: %s
`, envFile1, envFile2)
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

	// Verify env_from_file was resolved for both mods
	mods, ok := result["mods"].([]any)
	if !ok || len(mods) != 2 {
		t.Fatalf("expected 2 mods in array, got %v", result["mods"])
	}

	mod0 := mods[0].(map[string]any)
	env0 := mod0["env"].(map[string]any)
	if token, ok := env0["TOKEN"].(string); !ok || token != "secret-token-1" {
		t.Errorf("expected mods[0].env.TOKEN=secret-token-1, got %v", env0["TOKEN"])
	}

	mod1 := mods[1].(map[string]any)
	env1 := mod1["env"].(map[string]any)
	if token, ok := env1["TOKEN"].(string); !ok || token != "secret-token-2" {
		t.Errorf("expected mods[1].env.TOKEN=secret-token-2, got %v", env1["TOKEN"])
	}

	// Verify env_from_file was removed after resolution (clean spec)
	if _, exists := mod0["env_from_file"]; exists {
		t.Errorf("expected env_from_file to be removed from mods[0]")
	}
	if _, exists := mod1["env_from_file"]; exists {
		t.Errorf("expected env_from_file to be removed from mods[1]")
	}
}
