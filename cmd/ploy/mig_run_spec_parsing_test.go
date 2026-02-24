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
// Uses the canonical single-step format with steps[].
func TestBuildSpecPayloadFromYAML(t *testing.T) {
	// Create a temporary YAML spec file using canonical single-step format.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mod:latest
    retain_container: true
env:
  KEY1: value1
  KEY2: value2
build_gate:
  healing:
    retries: 1
    image: docker.io/test/healer:latest
  router:
    image: docker.io/test/router:latest
gitlab_domain: gitlab.example.com
mr_on_success: true
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	buildGate, ok := result["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate in payload")
	}
	if _, ok := buildGate["healing"].(map[string]any); !ok {
		t.Errorf("expected build_gate.healing in payload")
	}

	// Verify top-level settings (canonical single-step format).
	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0] in payload, got %T %v", result["steps"], result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if img, ok := step0["image"].(string); !ok || img != "docker.io/test/mod:latest" {
		t.Errorf("expected steps[0].image=docker.io/test/mod:latest, got %v", step0["image"])
	}
	if env, ok := result["env"].(map[string]any); ok {
		if env["KEY1"] != "value1" || env["KEY2"] != "value2" {
			t.Errorf("expected env with KEY1/KEY2, got %v", env)
		}
	} else {
		t.Errorf("expected env in payload")
	}
	if retain, ok := step0["retain_container"].(bool); !ok || !retain {
		t.Errorf("expected steps[0].retain_container=true, got %v", step0["retain_container"])
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

func TestBuildSpecPayloadFromYAML_BuildGateStackPrePost(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mod:latest
build_gate:
  enabled: true
  pre:
    stack:
      enabled: true
      language: java
      release: 11
      default: true
  post:
    stack:
      enabled: true
      language: java
      release: "17"
      default: true
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	buildGate, ok := result["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate in payload")
	}
	pre, ok := buildGate["pre"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.pre in payload")
	}
	preStack, ok := pre["stack"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.pre.stack in payload")
	}
	if preStack["language"] != "java" {
		t.Fatalf("expected build_gate.pre.stack.language=java, got %v", preStack["language"])
	}
	if preStack["default"] != true {
		t.Fatalf("expected build_gate.pre.stack.default=true, got %v", preStack["default"])
	}
	if rel, ok := preStack["release"].(float64); !ok || rel != 11 {
		t.Fatalf("expected build_gate.pre.stack.release=11, got %T %v", preStack["release"], preStack["release"])
	}

	post, ok := buildGate["post"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.post in payload")
	}
	postStack, ok := post["stack"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.post.stack in payload")
	}
	if postStack["release"] != "17" {
		t.Fatalf("expected build_gate.post.stack.release=\"17\", got %v", postStack["release"])
	}
}

// TestBuildSpecPayloadFromJSON verifies that buildSpecPayload correctly parses
// a JSON spec file and produces the expected payload structure.
func TestBuildSpecPayloadFromJSON(t *testing.T) {
	// Create a temporary JSON spec file
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.json")
	specContent := `{
  "steps": [{"image": "docker.io/test/mod:latest"}],
  "env": {
    "KEY1": "value1"
  },
  "build_gate": {
    "healing": {
      "retries": 2,
      "image": "docker.io/test/healer:latest"
    },
    "router": {
      "image": "docker.io/test/router:latest"
    }
  }
}`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify the spec contains build_gate.healing
	buildGate, ok := result["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate in payload")
	}
	healing, ok := buildGate["healing"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.healing in payload")
	}
	if retries, ok := healing["retries"].(float64); !ok || retries != 2 {
		t.Errorf("expected build_gate.healing.retries=2, got %v", healing["retries"])
	}
}

// TestBuildSpecPayloadCommand_MergesWithoutCLI verifies that when a spec defines
// command and no --job-command CLI flag is passed, the spec command is preserved.
// Uses canonical single-step format with top-level fields.
func TestBuildSpecPayloadCommand_MergesWithoutCLI(t *testing.T) {
	// Spec defines top-level command; CLI does not pass --job-command.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := `
steps:
  - image: docker.io/test/mod:latest
    command: ["/bin/sh", "-lc", "echo hi"]
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	steps, ok := out["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0], got %v", out["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	// Verify steps[0].command is preserved.
	cmd, ok := step0["command"].([]any)
	if !ok || len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-lc" || cmd[2] != "echo hi" {
		t.Fatalf("expected steps[0].command array preserved, got %v", step0["command"])
	}
}

// TestBuildSpecPayloadCommand_CLIOverridesJSON verifies that a CLI-provided
// JSON array command overrides the spec-defined command.
// Uses canonical single-step format with top-level fields.
func TestBuildSpecPayloadCommand_CLIOverridesJSON(t *testing.T) {
	// Spec defines top-level command; CLI provides JSON array to override.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := `
steps:
  - image: docker.io/test/mod:latest
    command: ["echo", "spec"]
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	payload, err := buildSpecPayload(specPath, nil, "", false, `["echo","cli"]`, "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	steps, ok := out["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0], got %v", out["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	// Verify steps[0].command is overridden by CLI.
	cmd, ok := step0["command"].([]any)
	if !ok || len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "cli" {
		t.Fatalf("expected steps[0].command overridden to [echo cli], got %v", step0["command"])
	}
}

// TestBuildSpecPayloadInvalidFile verifies that buildSpecPayload returns an error
// when attempting to read a non-existent spec file.
func TestBuildSpecPayloadInvalidFile(t *testing.T) {
	// Non-existent file should error
	_, err := buildSpecPayload("/nonexistent/path/spec.yaml", nil, "", false, "", "", "", false, false)
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

	_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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
		"docker.io/test/mod:latest",
		false,
		`["/bin/sh", "-c", "echo test"]`,
		"",
		"",
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

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0], got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if cmd, ok := step0["command"].([]any); ok {
		if len(cmd) != 3 {
			t.Errorf("expected command array length 3, got %d", len(cmd))
		}
		if cmd[0] != "/bin/sh" {
			t.Errorf("expected command[0]=/bin/sh, got %v", cmd[0])
		}
	} else {
		t.Errorf("expected command as array, got %T", step0["command"])
	}
}

// TestBuildSpecPayloadCommandString verifies that a CLI-provided command
// as a plain string is correctly stored in the payload.
func TestBuildSpecPayloadCommandString(t *testing.T) {
	// Test command as plain string
	payload, err := buildSpecPayload(
		"",
		nil,
		"docker.io/test/mod:latest",
		false,
		"echo test",
		"",
		"",
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

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0], got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if cmd, ok := step0["command"].(string); !ok || cmd != "echo test" {
		t.Errorf("expected command string 'echo test', got %v", step0["command"])
	}
}

// TestBuildSpecPayloadContainsBuildGateHealing verifies that complex nested
// structures (like build_gate_healing with retries and mod fields) are
// correctly parsed from YAML and preserved in the payload.
func TestBuildSpecPayloadContainsBuildGateHealing(t *testing.T) {
	// Test that build_gate.healing is preserved when present in spec
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mod:latest
build_gate:
  healing:
    retries: 2
    image: docker.io/test/healer:latest
    command: "heal.sh"
    env:
      HEALING_MODE: auto
  router:
    image: docker.io/test/router:latest
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	buildGate, ok := result["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate in payload")
	}
	healing, ok := buildGate["healing"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.healing in payload")
	}

	// Verify retries
	if retries, ok := healing["retries"].(float64); !ok || retries != 2 {
		t.Errorf("expected build_gate.healing.retries=2, got %v", healing["retries"])
	}

	// Verify flattened healing fields (no "mod" key)
	if img, ok := healing["image"].(string); !ok || img != "docker.io/test/healer:latest" {
		t.Errorf("expected healing.image=docker.io/test/healer:latest, got %v", healing["image"])
	}
	if cmd, ok := healing["command"].(string); !ok || cmd != "heal.sh" {
		t.Errorf("expected healing.command=heal.sh, got %v", healing["command"])
	}
	if env, ok := healing["env"].(map[string]any); ok {
		if mode, _ := env["HEALING_MODE"].(string); mode != "auto" {
			t.Errorf("expected healing.env.HEALING_MODE=auto, got %v", mode)
		}
	} else {
		t.Errorf("expected healing.env to be a map")
	}
	// retain_container=false → omitted from serialized output (omitempty).
	if _, ok := healing["retain_container"]; ok {
		t.Errorf("expected retain_container to be omitted (false → omitempty), got %v", healing["retain_container"])
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
steps:
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
  healing:
    retries: 1
    image: docker.io/test/healer:latest
  router:
    image: docker.io/test/router:latest
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify steps[] array exists and has 3 entries
	steps, ok := result["steps"].([]any)
	if !ok {
		t.Fatalf("expected steps array in payload, got %T", result["steps"])
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps in array, got %d", len(steps))
	}

	// Verify first mod step
	mod0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if img, ok := mod0["image"].(string); !ok || img != "docker.io/test/mod-step1:latest" {
		t.Errorf("expected steps[0].image=docker.io/test/mod-step1:latest, got %v", mod0["image"])
	}
	env0, ok := mod0["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0].env to be map, got %T", mod0["env"])
	}
	if step, ok := env0["STEP"].(string); !ok || step != "1" {
		t.Errorf("expected steps[0].env.STEP=1, got %v", env0["STEP"])
	}

	// Verify second mod step
	mod1, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[1] to be map, got %T", steps[1])
	}
	if img, ok := mod1["image"].(string); !ok || img != "docker.io/test/mod-step2:latest" {
		t.Errorf("expected steps[1].image=docker.io/test/mod-step2:latest, got %v", mod1["image"])
	}

	// Verify third mod step
	mod2, ok := steps[2].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[2] to be map, got %T", steps[2])
	}
	if img, ok := mod2["image"].(string); !ok || img != "docker.io/test/mod-step3:latest" {
		t.Errorf("expected steps[2].image=docker.io/test/mod-step3:latest, got %v", mod2["image"])
	}

	// Verify global build_gate and build_gate.healing are preserved
	buildGate, ok := result["build_gate"].(map[string]any)
	if !ok {
		t.Errorf("expected build_gate in payload")
	}
	if _, ok := buildGate["healing"].(map[string]any); !ok {
		t.Errorf("expected build_gate.healing in payload")
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
steps:
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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify env_from_file was resolved for both steps
	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected 2 steps in array, got %v", result["steps"])
	}

	mod0 := steps[0].(map[string]any)
	env0 := mod0["env"].(map[string]any)
	if token, ok := env0["TOKEN"].(string); !ok || token != "secret-token-1" {
		t.Errorf("expected steps[0].env.TOKEN=secret-token-1, got %v", env0["TOKEN"])
	}

	mod1 := steps[1].(map[string]any)
	env1 := mod1["env"].(map[string]any)
	if token, ok := env1["TOKEN"].(string); !ok || token != "secret-token-2" {
		t.Errorf("expected steps[1].env.TOKEN=secret-token-2, got %v", env1["TOKEN"])
	}

	// Verify env_from_file was removed after resolution (clean spec)
	if _, exists := mod0["env_from_file"]; exists {
		t.Errorf("expected env_from_file to be removed from steps[0]")
	}
	if _, exists := mod1["env_from_file"]; exists {
		t.Errorf("expected env_from_file to be removed from steps[1]")
	}
}

// TestBuildSpecPayload_CanonicalSingleStepWithOverrides verifies that single-step specs
// using the canonical format (top-level fields) work with CLI overrides.
// CLI overrides apply to single-step format but not to multi-step mods[] format.
func TestBuildSpecPayload_CanonicalSingleStepWithOverrides(t *testing.T) {
	t.Parallel()

	// Test canonical single-step format with CLI overrides.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "single.yaml")
	specContent := `
steps:
  - image: docker.io/test/base:v1
env:
  BASE_KEY: base_value
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// Apply CLI overrides (env, image, retain).
	payload, err := buildSpecPayload(
		specPath,
		[]string{"CLI_KEY=cli_value"},
		"docker.io/test/override:v2",
		true, // retain
		"",
		"",
		"",
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

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0] in payload, got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}

	// Image override applied.
	if img, ok := step0["image"].(string); !ok || img != "docker.io/test/override:v2" {
		t.Errorf("expected steps[0].image=docker.io/test/override:v2, got %v", step0["image"])
	}

	// Retain override applied.
	if retain, ok := step0["retain_container"].(bool); !ok || !retain {
		t.Errorf("expected steps[0].retain_container=true, got %v", step0["retain_container"])
	}

	// Env merged (CLI + spec).
	env, ok := result["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env to be present")
	}
	if base, ok := env["BASE_KEY"].(string); !ok || base != "base_value" {
		t.Errorf("expected env.BASE_KEY=base_value, got %v", env["BASE_KEY"])
	}
	if cli, ok := env["CLI_KEY"].(string); !ok || cli != "cli_value" {
		t.Errorf("expected env.CLI_KEY=cli_value, got %v", env["CLI_KEY"])
	}

	if _, exists := result["mods"]; exists {
		t.Errorf("expected legacy mods[] to be absent")
	}
}

// TestBuildSpecPayload_MultiStepIgnoresCLIOverrides verifies that when mods[]
// is present, CLI overrides are NOT applied to the multi-step mods array.
// Multi-step mods[] must be fully specified in the spec file; CLI flags only
// apply to single-mod format for backward compatibility.
func TestBuildSpecPayload_MultiStepIgnoresCLIOverrides(t *testing.T) {
	t.Parallel()

	// Test multi-step format with CLI overrides (which should be ignored).
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "multi.yaml")
	specContent := `
steps:
  - image: docker.io/test/step1:v1
    env:
      STEP: "1"
  - image: docker.io/test/step2:v1
    env:
      STEP: "2"
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// Attempt to apply CLI overrides (should be ignored for multi-step format).
	payload, err := buildSpecPayload(
		specPath,
		[]string{"CLI_KEY=cli_value"},
		"docker.io/test/override:v2",
		true,
		"",
		"",
		"",
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

	// Verify steps[] is present and unchanged.
	steps, ok := result["steps"].([]any)
	if !ok {
		t.Fatalf("expected steps[] array in payload")
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps in array, got %d", len(steps))
	}

	// Verify first step is unchanged (CLI overrides not applied).
	mod0 := steps[0].(map[string]any)
	if img, ok := mod0["image"].(string); !ok || img != "docker.io/test/step1:v1" {
		t.Errorf("expected steps[0].image=docker.io/test/step1:v1, got %v", mod0["image"])
	}
	if env0, ok := mod0["env"].(map[string]any); ok {
		// Only STEP should be present (not CLI_KEY).
		if len(env0) != 1 {
			t.Errorf("expected steps[0].env to have 1 key, got %d: %v", len(env0), env0)
		}
		if step, ok := env0["STEP"].(string); !ok || step != "1" {
			t.Errorf("expected steps[0].env.STEP=1, got %v", env0["STEP"])
		}
	}

	// Verify second step is unchanged.
	mod1 := steps[1].(map[string]any)
	if img, ok := mod1["image"].(string); !ok || img != "docker.io/test/step2:v1" {
		t.Errorf("expected steps[1].image=docker.io/test/step2:v1, got %v", mod1["image"])
	}

	// Verify top-level env override exists (applies to all steps).
	if topEnv, ok := result["env"].(map[string]any); ok {
		if cli, ok := topEnv["CLI_KEY"].(string); !ok || cli != "cli_value" {
			t.Errorf("expected env.CLI_KEY=cli_value, got %v", topEnv["CLI_KEY"])
		}
	} else {
		t.Fatalf("expected env in payload")
	}

	// Image/retain overrides are ignored for multi-step specs.
	if _, exists := result["image"]; exists {
		t.Errorf("expected top-level image to be absent")
	}
	if _, exists := result["retain_container"]; exists {
		t.Errorf("expected top-level retain_container to be absent")
	}
}
