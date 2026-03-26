package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// newMockBundleSrvForLoadSpec creates a mock bundle server for loadSpec tests.
func newMockBundleSrvForLoadSpec(t *testing.T) (*httptest.Server, *url.URL, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/spec-bundles" {
			data, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bundle_id": "test-bundle-id",
				"cid":       "bafytest",
				"digest":    "sha256:deadbeef",
				"size":      len(data),
			})
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return srv, u, srv.Client()
}

func TestLoadSpec_ResolvesStepAndRouterPreprocessing(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

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

	payload, err := loadSpec(context.Background(), base, client, specPath)
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

	// tmp_dir should be replaced with tmp_bundle after upload.
	if _, hasTmpDir := step["tmp_dir"]; hasTmpDir {
		t.Fatalf("expected steps[0].tmp_dir removed after bundle upload")
	}
	stepBundle, hasTmpBundle := step["tmp_bundle"].(map[string]any)
	if !hasTmpBundle {
		t.Fatalf("expected steps[0].tmp_bundle set after bundle upload")
	}
	if stepBundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("steps[0].tmp_bundle.bundle_id: got %q, want %q", stepBundle["bundle_id"], "test-bundle-id")
	}
	if entries, ok := stepBundle["entries"].([]any); !ok || len(entries) != 1 || entries[0] != "step.txt" {
		t.Errorf("steps[0].tmp_bundle.entries: got %v, want [step.txt]", stepBundle["entries"])
	}

	router := result["build_gate"].(map[string]any)["router"].(map[string]any)
	if _, ok := router["env_from_file"]; ok {
		t.Fatalf("expected router env_from_file removed after preprocessing")
	}
	routerEnv := router["env"].(map[string]any)
	if got, want := routerEnv["ROUTER_TOKEN"].(string), "router-token"; got != want {
		t.Fatalf("build_gate.router.env.ROUTER_TOKEN got %q, want %q", got, want)
	}

	// tmp_dir should be replaced with tmp_bundle.
	if _, hasTmpDir := router["tmp_dir"]; hasTmpDir {
		t.Fatalf("expected router.tmp_dir removed after bundle upload")
	}
	routerBundle, hasTmpBundle := router["tmp_bundle"].(map[string]any)
	if !hasTmpBundle {
		t.Fatalf("expected router.tmp_bundle set after bundle upload")
	}
	if routerBundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("router.tmp_bundle.bundle_id: got %q, want %q", routerBundle["bundle_id"], "test-bundle-id")
	}
	if entries, ok := routerBundle["entries"].([]any); !ok || len(entries) != 1 || entries[0] != "router.txt" {
		t.Errorf("router.tmp_bundle.entries: got %v, want [router.txt]", routerBundle["entries"])
	}
}

func TestLoadSpec_ResolvesHealingPreprocessing(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

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

	payload, err := loadSpec(context.Background(), base, client, specPath)
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

	// tmp_dir should be replaced with tmp_bundle.
	if _, hasTmpDir := infra["tmp_dir"]; hasTmpDir {
		t.Fatalf("expected infra.tmp_dir removed after bundle upload")
	}
	infraBundle, hasTmpBundle := infra["tmp_bundle"].(map[string]any)
	if !hasTmpBundle {
		t.Fatalf("expected infra.tmp_bundle set after bundle upload")
	}
	if infraBundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("infra.tmp_bundle.bundle_id: got %q, want %q", infraBundle["bundle_id"], "test-bundle-id")
	}
	if entries, ok := infraBundle["entries"].([]any); !ok || len(entries) != 1 || entries[0] != "healing.txt" {
		t.Errorf("infra.tmp_bundle.entries: got %v, want [healing.txt]", infraBundle["entries"])
	}
}

func TestLoadSpec_ExpandsEnvPlaceholders(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	t.Setenv("PLOY_TEST_LOADSPEC_TOKEN", "loadspectoken")

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
env:
  TOKEN: $PLOY_TEST_LOADSPEC_TOKEN
  URL: https://${PLOY_TEST_LOADSPEC_TOKEN}.example.test
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// No tmp_dir sections, so nil base/client is fine.
	payload, err := loadSpec(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("loadSpec() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	env := result["env"].(map[string]any)
	if got, want := env["TOKEN"].(string), "loadspectoken"; got != want {
		t.Fatalf("env.TOKEN got %q, want %q", got, want)
	}
	if got, want := env["URL"].(string), "https://loadspectoken.example.test"; got != want {
		t.Fatalf("env.URL got %q, want %q", got, want)
	}
}
