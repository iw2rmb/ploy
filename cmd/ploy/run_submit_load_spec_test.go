package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// newMockBundleSrvForLoadSpec creates a mock bundle server for loadSpec tests.
func newMockBundleSrvForLoadSpec(t *testing.T) (*httptest.Server, *url.URL, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spec-bundles" {
			if r.Method == http.MethodHead {
				// All probes miss in loadSpec tests (first-time upload).
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodPost {
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

func newCapturingBundleSrvForLoadSpec(t *testing.T) (*httptest.Server, *url.URL, *http.Client, map[string][]byte) {
	t.Helper()
	uploads := make(map[string][]byte)
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spec-bundles" {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodPost {
				data, _ := io.ReadAll(r.Body)
				hash := computeArchiveShortHash(data)
				mu.Lock()
				uploads[hash] = append([]byte(nil), data...)
				mu.Unlock()
				fullDigest := sha256.Sum256(data)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"bundle_id": "bundle-" + hash,
					"cid":       computeSpecBundleCID(data),
					"digest":    "sha256:" + hex.EncodeToString(fullDigest[:]),
					"size":      len(data),
				})
				return
			}
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return srv, u, srv.Client(), uploads
}

func extractSingleContentFileFromArchive(t *testing.T, archive []byte) []byte {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if hdr == nil || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Name != "content" {
			continue
		}
		payload, readErr := io.ReadAll(tr)
		if readErr != nil {
			t.Fatalf("read content entry: %v", readErr)
		}
		return payload
	}
	t.Fatal("archive missing content file")
	return nil
}

func TestLoadSpec_ResolvesStepHydraRecords(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	stepInFile := filepath.Join(tmpDir, "step-config.txt")
	specPath := filepath.Join(tmpDir, "spec.yaml")

	if err := os.WriteFile(stepInFile, []byte("step-config-data"), 0o644); err != nil {
		t.Fatalf("write step in file: %v", err)
	}

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
    envs:
      STEP_TOKEN: step-token
    in:
      - ` + stepInFile + `:config.txt
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
	stepEnvs := step["envs"].(map[string]any)
	if got, want := stepEnvs["STEP_TOKEN"].(string), "step-token"; got != want {
		t.Fatalf("steps[0].envs.STEP_TOKEN got %q, want %q", got, want)
	}

	// in entries should be compiled to canonical shortHash:/in/dst form.
	stepIn, ok := step["in"].([]any)
	if !ok || len(stepIn) != 1 {
		t.Fatalf("expected steps[0].in with 1 entry, got %v", step["in"])
	}
	stepInEntry, ok := stepIn[0].(string)
	if !ok {
		t.Fatalf("expected steps[0].in[0] to be string, got %T", stepIn[0])
	}
	if !strings.Contains(stepInEntry, ":/in/config.txt") {
		t.Errorf("expected steps[0].in[0] to contain :/in/config.txt, got %q", stepInEntry)
	}
}

func TestLoadSpec_ResolvesHealHydraRecords(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	healingInFile := filepath.Join(tmpDir, "healing-config.txt")
	specPath := filepath.Join(tmpDir, "spec.yaml")

	if err := os.WriteFile(healingInFile, []byte("healing-config-data"), 0o644); err != nil {
		t.Fatalf("write healing in file: %v", err)
	}

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    retries: 1
    image: docker.io/test/healer:latest
    envs:
      HEALING_TOKEN: healing-token
    in:
      - ` + healingInFile + `:healing-config.txt
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

	heal := result["build_gate"].(map[string]any)["heal"].(map[string]any)
	healingEnvs := heal["envs"].(map[string]any)
	if got, want := healingEnvs["HEALING_TOKEN"].(string), "healing-token"; got != want {
		t.Fatalf("build_gate.heal.envs.HEALING_TOKEN got %q, want %q", got, want)
	}

	healingIn, ok := heal["in"].([]any)
	if !ok || len(healingIn) != 1 {
		t.Fatalf("expected heal.in with 1 entry, got %v", heal["in"])
	}
	healingInEntry, ok := healingIn[0].(string)
	if !ok {
		t.Fatalf("expected heal.in[0] to be string, got %T", healingIn[0])
	}
	if !strings.Contains(healingInEntry, ":/in/healing-config.txt") {
		t.Errorf("expected heal.in[0] to contain :/in/healing-config.txt, got %q", healingInEntry)
	}
}

func TestLoadSpec_ExpandsEnvPlaceholders(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	t.Setenv("PLOY_TEST_LOADSPEC_TOKEN", "loadspectoken")

	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
envs:
  TOKEN: $PLOY_TEST_LOADSPEC_TOKEN
  URL: https://${PLOY_TEST_LOADSPEC_TOKEN}.example.test
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// No Hydra file records, so nil base/client is fine.
	payload, err := loadSpec(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("loadSpec() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	envs := result["envs"].(map[string]any)
	if got, want := envs["TOKEN"].(string), "loadspectoken"; got != want {
		t.Fatalf("envs.TOKEN got %q, want %q", got, want)
	}
	if got, want := envs["URL"].(string), "https://loadspectoken.example.test"; got != want {
		t.Fatalf("envs.URL got %q, want %q", got, want)
	}
}

func TestLoadSpec_ExpandsImagePlaceholders(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	t.Setenv("PLOY_TEST_LOADSPEC_IMAGE", "docker.io/test/amata:latest")
	t.Setenv("PLOY_TEST_LOADSPEC_STEP_DEFAULT", "docker.io/test/default-step:latest")

	spec := []byte(`
steps:
  - image:
      default: $PLOY_TEST_LOADSPEC_STEP_DEFAULT
      java-gradle: ${PLOY_TEST_LOADSPEC_IMAGE}
build_gate:
  heal:
    retries: 1
    image: ${PLOY_TEST_LOADSPEC_IMAGE}
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := loadSpec(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("loadSpec() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	step := result["steps"].([]any)[0].(map[string]any)
	image := step["image"].(map[string]any)
	if got, want := image["default"].(string), "docker.io/test/default-step:latest"; got != want {
		t.Fatalf("steps[0].image.default got %q, want %q", got, want)
	}
	if got, want := image["java-gradle"].(string), "docker.io/test/amata:latest"; got != want {
		t.Fatalf("steps[0].image.java-gradle got %q, want %q", got, want)
	}

	heal := result["build_gate"].(map[string]any)["heal"].(map[string]any)
	if got, want := heal["image"].(string), "docker.io/test/amata:latest"; got != want {
		t.Fatalf("build_gate.heal.image got %q, want %q", got, want)
	}
}

func TestLoadSpec_IncludeFragmentNormalizesRelativeHydraSources(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	fragmentsDir := filepath.Join(tmpDir, "fragments")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatalf("mkdir fragments: %v", err)
	}

	// Source file path is intentionally relative in the included fragment.
	if err := os.WriteFile(filepath.Join(fragmentsDir, "heal-config.txt"), []byte("heal-config-data"), 0o644); err != nil {
		t.Fatalf("write heal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fragmentsDir, "heal.fragment.yaml"), []byte(`
heal:
  image: docker.io/test/healer:latest
  in:
    - ./heal-config.txt:heal-config.txt
`), 0o644); err != nil {
		t.Fatalf("write heal fragment: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    <<: !include ./fragments/heal.fragment.yaml#/heal
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

	heal := result["build_gate"].(map[string]any)["heal"].(map[string]any)
	healIn := heal["in"].([]any)
	entry, ok := healIn[0].(string)
	if !ok {
		t.Fatalf("expected heal.in[0] string, got %T", healIn[0])
	}
	if !strings.Contains(entry, ":/in/heal-config.txt") {
		t.Fatalf("heal.in[0] = %q, want canonical /in destination", entry)
	}
}

func TestLoadSpec_CompilesLocalHookDirectoryToHashesAndBundleMap(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "hooks", "a"), 0o755); err != nil {
		t.Fatalf("mkdir hooks a: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "hooks", "b"), 0o755); err != nil {
		t.Fatalf("mkdir hooks b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "hooks", "a", "hook.yaml"), []byte("id: a\nsteps:\n  - image: test:latest\n"), 0o644); err != nil {
		t.Fatalf("write hook a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "hooks", "b", "hook.yaml"), []byte("id: b\nsteps:\n  - image: test:latest\n"), 0o644); err != nil {
		t.Fatalf("write hook b: %v", err)
	}
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
hooks:
  - ./hooks
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

	hooksRaw, ok := result["hooks"].([]any)
	if !ok || len(hooksRaw) != 2 {
		t.Fatalf("hooks=%v, want 2 compiled hook hashes", result["hooks"])
	}
	re := regexp.MustCompile(`^[0-9a-f]{12}$`)
	bm, ok := result["bundle_map"].(map[string]any)
	if !ok {
		t.Fatalf("bundle_map type=%T, want map[string]any", result["bundle_map"])
	}
	for i, item := range hooksRaw {
		hash, ok := item.(string)
		if !ok {
			t.Fatalf("hooks[%d] type=%T, want string", i, item)
		}
		if !re.MatchString(hash) {
			t.Fatalf("hooks[%d]=%q, want short hash", i, hash)
		}
		if got, ok := bm[hash].(string); !ok || got == "" {
			t.Fatalf("bundle_map[%q]=%v, want non-empty bundle id", hash, bm[hash])
		}
	}
}

func TestLoadSpec_CompilesLocalHookFileToHash(t *testing.T) {
	_, base, client := newMockBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	hookFile := filepath.Join(tmpDir, "hook.yaml")
	if err := os.WriteFile(hookFile, []byte("id: file-hook\nsteps:\n  - image: test:latest\n"), 0o644); err != nil {
		t.Fatalf("write hook file: %v", err)
	}
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
hooks:
  - ./hook.yaml
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
	hooks, ok := result["hooks"].([]any)
	if !ok || len(hooks) != 1 {
		t.Fatalf("hooks=%v, want one compiled hook hash", result["hooks"])
	}
	hash, _ := hooks[0].(string)
	if ok := regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(hash); !ok {
		t.Fatalf("hooks[0]=%q, want short hash", hash)
	}
}

func TestLoadSpec_LocalHookSourcesRequireServerBaseAndClient(t *testing.T) {
	tmpDir := t.TempDir()
	hookFile := filepath.Join(tmpDir, "hook.yaml")
	if err := os.WriteFile(hookFile, []byte("id: file-hook\nsteps:\n  - image: test:latest\n"), 0o644); err != nil {
		t.Fatalf("write hook file: %v", err)
	}
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
hooks:
  - ./hook.yaml
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := loadSpec(context.Background(), nil, nil, specPath)
	if err == nil {
		t.Fatal("expected error for local hook sources without base/client")
	}
	if !strings.Contains(err.Error(), "local hook sources found but no server base URL available for upload") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSpec_LocalHookManifestCompilesHydraAndExpandsPlaceholders(t *testing.T) {
	_, base, client, uploads := newCapturingBundleSrvForLoadSpec(t)

	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONTAINER_REGISTRY", "ghcr.io/example")
	t.Setenv("PLOY_VERSION", "v1.2.3")
	t.Setenv("HOOK_TOKEN", "from-env")

	if err := os.WriteFile(filepath.Join(tmpDir, "amata.yaml"), []byte("version: amata/v1\nname: test\nentry: main\n"), 0o644); err != nil {
		t.Fatalf("write amata file: %v", err)
	}
	hookPath := filepath.Join(tmpDir, "hook.yaml")
	hookSpec := []byte(`
id: local-hook
steps:
  - image: $PLOY_CONTAINER_REGISTRY/amata:$PLOY_VERSION
    envs:
      TOKEN: $HOOK_TOKEN
    in:
      - ./amata.yaml:amata.yaml
`)
	if err := os.WriteFile(hookPath, hookSpec, 0o644); err != nil {
		t.Fatalf("write hook file: %v", err)
	}
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - image: docker.io/test/mig:latest
hooks:
  - ./hook.yaml
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
	hooksRaw, ok := result["hooks"].([]any)
	if !ok || len(hooksRaw) != 1 {
		t.Fatalf("hooks=%v, want one compiled hook hash", result["hooks"])
	}
	hookHash, ok := hooksRaw[0].(string)
	if !ok || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(hookHash) {
		t.Fatalf("hooks[0]=%v, want short hash string", hooksRaw[0])
	}
	if _, ok := uploads[hookHash]; !ok {
		t.Fatalf("expected uploaded hook bundle for hash %q", hookHash)
	}

	hookPayload := extractSingleContentFileFromArchive(t, uploads[hookHash])
	var hookMap map[string]any
	if err := json.Unmarshal(hookPayload, &hookMap); err != nil {
		t.Fatalf("unmarshal canonical hook payload: %v", err)
	}
	steps, ok := hookMap["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("canonical hook steps=%v, want 1", hookMap["steps"])
	}
	step := steps[0].(map[string]any)
	if got, want := step["image"], "ghcr.io/example/amata:v1.2.3"; got != want {
		t.Fatalf("hook step image=%v, want %q", got, want)
	}
	envs, ok := step["envs"].(map[string]any)
	if !ok {
		t.Fatalf("hook step envs type=%T, want map[string]any", step["envs"])
	}
	if got, want := envs["TOKEN"], "from-env"; got != want {
		t.Fatalf("hook step env TOKEN=%v, want %q", got, want)
	}
	inEntries, ok := step["in"].([]any)
	if !ok || len(inEntries) != 1 {
		t.Fatalf("hook step in=%v, want one canonical entry", step["in"])
	}
	inEntry, ok := inEntries[0].(string)
	if !ok {
		t.Fatalf("hook step in[0] type=%T, want string", inEntries[0])
	}
	if !strings.Contains(inEntry, ":/in/amata.yaml") {
		t.Fatalf("hook step in[0]=%q, want canonical /in path", inEntry)
	}
	if strings.Contains(inEntry, "./amata.yaml:") {
		t.Fatalf("hook step in[0]=%q, authoring source must be compiled out", inEntry)
	}
}
