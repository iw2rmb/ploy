package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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

func TestBuildSpecPayloadCLIOverrides(t *testing.T) {
	// Create a temporary YAML spec file with some defaults
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
image: docker.io/test/mod:v1
env:
  KEY1: from_spec
  KEY2: value2
gitlab_domain: gitlab.com
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// CLI overrides should take precedence
	modEnvs := []string{"KEY1=from_cli", "KEY3=new_value"}
	payload, err := buildSpecPayload(
		specPath,
		modEnvs,
		"docker.io/test/mod:v2", // override image
		true,                    // retain
		"",
		"glpat-test",         // gitlab_pat
		"gitlab.example.com", // override domain
		true,                 // mr_success
		false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify CLI override for image
	if img, ok := result["image"].(string); !ok || img != "docker.io/test/mod:v2" {
		t.Errorf("expected image=docker.io/test/mod:v2 (CLI override), got %v", result["image"])
	}

	// Verify CLI override for env
	if env, ok := result["env"].(map[string]any); ok {
		if v, ok := env["KEY1"].(string); !ok || v != "from_cli" {
			t.Errorf("expected env.KEY1=from_cli (CLI override), got %v", env["KEY1"])
		}
		if v, ok := env["KEY2"].(string); !ok || v != "value2" {
			t.Errorf("expected env.KEY2=value2 (from spec), got %v", env["KEY2"])
		}
		if v, ok := env["KEY3"].(string); !ok || v != "new_value" {
			t.Errorf("expected env.KEY3=new_value (CLI override), got %v", env["KEY3"])
		}
	} else {
		t.Errorf("expected env in payload")
	}

	// Verify CLI override for retain_container
	if retain, ok := result["retain_container"].(bool); !ok || !retain {
		t.Errorf("expected retain_container=true (CLI override), got %v", result["retain_container"])
	}

	// Verify CLI override for gitlab_domain
	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
		t.Errorf("expected gitlab_domain=gitlab.example.com (CLI override), got %v", result["gitlab_domain"])
	}

	// Verify gitlab_pat from CLI
	if pat, ok := result["gitlab_pat"].(string); !ok || pat != "glpat-test" {
		t.Errorf("expected gitlab_pat=glpat-test (CLI), got %v", result["gitlab_pat"])
	}

	// Verify mr_on_success from CLI
	if success, ok := result["mr_on_success"].(bool); !ok || !success {
		t.Errorf("expected mr_on_success=true (CLI), got %v", result["mr_on_success"])
	}
}

func TestBuildSpecPayloadNoSpec(t *testing.T) {
	// No spec file, only CLI flags
	payload, err := buildSpecPayload(
		"",
		[]string{"KEY1=value1"},
		"docker.io/test/mod:latest",
		false,
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

	// Verify CLI-only values
	if img, ok := result["image"].(string); !ok || img != "docker.io/test/mod:latest" {
		t.Errorf("expected image=docker.io/test/mod:latest, got %v", result["image"])
	}

	if env, ok := result["env"].(map[string]any); ok {
		if v, ok := env["KEY1"].(string); !ok || v != "value1" {
			t.Errorf("expected env.KEY1=value1, got %v", env["KEY1"])
		}
	} else {
		t.Errorf("expected env in payload")
	}
}

func TestBuildSpecPayloadEmpty(t *testing.T) {
	// No spec file and no CLI overrides
	payload, err := buildSpecPayload("", nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	if payload != nil {
		t.Errorf("expected nil payload when no spec or overrides, got %v", payload)
	}
}

func TestBuildSpecPayloadInvalidFile(t *testing.T) {
	// Non-existent file should error
	_, err := buildSpecPayload("/nonexistent/path/spec.yaml", nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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
