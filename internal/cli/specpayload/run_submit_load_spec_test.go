package specpayload

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
	"strings"
	"sync"
	"testing"
)

// newBundleSrvForLoadSpec creates a bundle server for Load tests. It always
// records uploads (keyed by short hash) so any caller can inspect captures.
func newBundleSrvForLoadSpec(t *testing.T) (*url.URL, *http.Client, map[string][]byte) {
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
	return u, srv.Client(), uploads
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
	base, client, _ := newBundleSrvForLoadSpec(t)

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

	payload, err := Load(context.Background(), base, client, specPath)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
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

func TestLoadSpec_NormalizesStepInObjectEntriesToInFrom(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "spec.yaml")
	spec := []byte(`
steps:
  - name: extract-usage
    image: docker.io/test/extract:latest
  - name: compose-deprecations
    image: docker.io/test/compose:latest
    in:
      - from: extract-usage://out/dependency-usage.nofilter.json
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := Load(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("steps=%v, want 2 entries", result["steps"])
	}

	second, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("steps[1] type=%T, want map[string]any", steps[1])
	}

	inVals, ok := second["in"].([]any)
	if !ok {
		t.Fatalf("steps[1].in type=%T, want []any", second["in"])
	}
	if len(inVals) != 0 {
		t.Fatalf("steps[1].in len=%d, want 0", len(inVals))
	}

	inFromVals, ok := second["in_from"].([]any)
	if !ok || len(inFromVals) != 1 {
		t.Fatalf("steps[1].in_from=%v, want 1 entry", second["in_from"])
	}
	ref, ok := inFromVals[0].(map[string]any)
	if !ok {
		t.Fatalf("steps[1].in_from[0] type=%T, want map[string]any", inFromVals[0])
	}
	if got, want := ref["from"], "extract-usage://out/dependency-usage.nofilter.json"; got != want {
		t.Fatalf("steps[1].in_from[0].from=%v, want %q", got, want)
	}
	if got, want := ref["to"], "/in/dependency-usage.nofilter.json"; got != want {
		t.Fatalf("steps[1].in_from[0].to=%v, want %q", got, want)
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
	payload, err := Load(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
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
	t.Setenv("PLOY_TEST_LOADSPEC_IMAGE", "docker.io/test/step:latest")
	t.Setenv("PLOY_TEST_LOADSPEC_STEP_DEFAULT", "docker.io/test/default-step:latest")

	spec := []byte(`
steps:
  - image:
      default: $PLOY_TEST_LOADSPEC_STEP_DEFAULT
      java-gradle: ${PLOY_TEST_LOADSPEC_IMAGE}
`)
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := Load(context.Background(), nil, nil, specPath)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
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
	if got, want := image["java-gradle"].(string), "docker.io/test/step:latest"; got != want {
		t.Fatalf("steps[0].image.java-gradle got %q, want %q", got, want)
	}
}
