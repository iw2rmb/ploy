package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func decodeTmpFilePayload(t *testing.T, raw any) contracts.TmpFilePayload {
	t.Helper()

	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tmp_dir entry: %v", err)
	}
	var payload contracts.TmpFilePayload
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal tmp_dir entry: %v", err)
	}
	return payload
}

func TestLoadSpec_ResolvesStepAndRouterPreprocessing(t *testing.T) {
	tmpDir := t.TempDir()
	stepEnvPath := filepath.Join(tmpDir, "step.env")
	stepTmpPath := filepath.Join(tmpDir, "step.txt")
	routerEnvPath := filepath.Join(tmpDir, "router.env")
	routerTmpPath := filepath.Join(tmpDir, "router.txt")
	specPath := filepath.Join(tmpDir, "spec.yaml")

	if err := os.WriteFile(stepEnvPath, []byte("step-token"), 0o644); err != nil {
		t.Fatalf("write step env file: %v", err)
	}
	if err := os.WriteFile(stepTmpPath, []byte("step-tmp"), 0o644); err != nil {
		t.Fatalf("write step tmp file: %v", err)
	}
	if err := os.WriteFile(routerEnvPath, []byte("router-token"), 0o644); err != nil {
		t.Fatalf("write router env file: %v", err)
	}
	if err := os.WriteFile(routerTmpPath, []byte("router-tmp"), 0o644); err != nil {
		t.Fatalf("write router tmp file: %v", err)
	}

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
    env_from_file:
      STEP_TOKEN: ` + stepEnvPath + `
    tmp_dir:
      - name: step.txt
        path: ` + stepTmpPath + `
build_gate:
  router:
    image: docker.io/test/router:latest
    env_from_file:
      ROUTER_TOKEN: ` + routerEnvPath + `
    tmp_dir:
      - name: router.txt
        path: ` + routerTmpPath + `
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := loadSpec(specPath)
	if err != nil {
		t.Fatalf("loadSpec() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	step := result["steps"].([]any)[0].(map[string]any)
	if _, ok := step["env_from_file"]; ok {
		t.Fatalf("expected step env_from_file removed after preprocessing")
	}
	stepEnv := step["env"].(map[string]any)
	if got, want := stepEnv["STEP_TOKEN"].(string), "step-token"; got != want {
		t.Fatalf("steps[0].env.STEP_TOKEN got %q, want %q", got, want)
	}
	stepTmpDir := step["tmp_dir"].([]any)
	stepTmp := decodeTmpFilePayload(t, stepTmpDir[0])
	if got, want := string(stepTmp.Content), "step-tmp"; got != want {
		t.Fatalf("steps[0].tmp_dir[0].content got %q, want %q", got, want)
	}

	router := result["build_gate"].(map[string]any)["router"].(map[string]any)
	if _, ok := router["env_from_file"]; ok {
		t.Fatalf("expected router env_from_file removed after preprocessing")
	}
	routerEnv := router["env"].(map[string]any)
	if got, want := routerEnv["ROUTER_TOKEN"].(string), "router-token"; got != want {
		t.Fatalf("build_gate.router.env.ROUTER_TOKEN got %q, want %q", got, want)
	}
	routerTmpDir := router["tmp_dir"].([]any)
	routerTmp := decodeTmpFilePayload(t, routerTmpDir[0])
	if got, want := string(routerTmp.Content), "router-tmp"; got != want {
		t.Fatalf("build_gate.router.tmp_dir[0].content got %q, want %q", got, want)
	}
}

func TestLoadSpec_ResolvesHealingPreprocessing(t *testing.T) {
	tmpDir := t.TempDir()
	healingEnvPath := filepath.Join(tmpDir, "healing.env")
	healingTmpPath := filepath.Join(tmpDir, "healing.txt")
	specPath := filepath.Join(tmpDir, "spec.yaml")

	if err := os.WriteFile(healingEnvPath, []byte("healing-token"), 0o644); err != nil {
		t.Fatalf("write healing env file: %v", err)
	}
	if err := os.WriteFile(healingTmpPath, []byte("healing-tmp"), 0o644); err != nil {
		t.Fatalf("write healing tmp file: %v", err)
	}

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router:
    image: docker.io/test/router:latest
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/healer:latest
        env_from_file:
          HEALING_TOKEN: ` + healingEnvPath + `
        tmp_dir:
          - name: healing.txt
            path: ` + healingTmpPath + `
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := loadSpec(specPath)
	if err != nil {
		t.Fatalf("loadSpec() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	infra := result["build_gate"].(map[string]any)["healing"].(map[string]any)["by_error_kind"].(map[string]any)["infra"].(map[string]any)
	if _, ok := infra["env_from_file"]; ok {
		t.Fatalf("expected healing env_from_file removed after preprocessing")
	}
	healingEnv := infra["env"].(map[string]any)
	if got, want := healingEnv["HEALING_TOKEN"].(string), "healing-token"; got != want {
		t.Fatalf("build_gate.healing.by_error_kind.infra.env.HEALING_TOKEN got %q, want %q", got, want)
	}
	healingTmpDir := infra["tmp_dir"].([]any)
	healingTmp := decodeTmpFilePayload(t, healingTmpDir[0])
	if got, want := string(healingTmp.Content), "healing-tmp"; got != want {
		t.Fatalf("build_gate.healing.by_error_kind.infra.tmp_dir[0].content got %q, want %q", got, want)
	}
}
