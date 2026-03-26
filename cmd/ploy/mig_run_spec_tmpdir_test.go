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
	"strings"
	"testing"
)

// newMockBundleServer creates a test HTTP server that handles POST /v1/spec-bundles.
// It captures the last uploaded bytes for inspection and returns a fixed response.
func newMockBundleServer(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	var lastUpload []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/spec-bundles" {
			data, _ := io.ReadAll(r.Body)
			lastUpload = data
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
	return srv, &lastUpload
}

// parsedBundleBase parses the test server URL into a *url.URL.
func parsedBundleBase(t *testing.T, srv *httptest.Server) *url.URL {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u
}

// TestBuildSpecPayload_TmpDir_Steps verifies that tmp_dir entries in steps[]
// are archived, uploaded, and replaced with a tmp_bundle reference.
func TestBuildSpecPayload_TmpDir_Steps(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{"key":"value"}`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: config.json
        path: ` + configFile + `
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	steps := result["steps"].([]any)
	step0 := steps[0].(map[string]any)

	if _, hasTmpDir := step0["tmp_dir"]; hasTmpDir {
		t.Errorf("expected 'tmp_dir' to be removed from step after bundle upload")
	}

	tmpBundle, hasTmpBundle := step0["tmp_bundle"]
	if !hasTmpBundle {
		t.Fatalf("expected 'tmp_bundle' to be set in step after bundle upload")
	}

	bundle := tmpBundle.(map[string]any)
	if bundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("expected bundle_id=test-bundle-id, got %q", bundle["bundle_id"])
	}
	if bundle["cid"] != "bafytest" {
		t.Errorf("expected cid=bafytest, got %q", bundle["cid"])
	}
	if bundle["digest"] != "sha256:deadbeef" {
		t.Errorf("expected digest=sha256:deadbeef, got %q", bundle["digest"])
	}

	entries, ok := bundle["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries to be []any, got %T", bundle["entries"])
	}
	if len(entries) != 1 || entries[0] != "config.json" {
		t.Errorf("expected entries=[config.json], got %v", entries)
	}
}

// TestBuildSpecPayload_TmpDir_Router verifies that tmp_dir entries in build_gate.router
// are archived, uploaded, and replaced with a tmp_bundle reference.
func TestBuildSpecPayload_TmpDir_Router(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	promptFile := filepath.Join(tmpDir, "prompt.txt")
	promptContent := "Analyze the build log."
	if err := os.WriteFile(promptFile, []byte(promptContent), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/heal:latest
  router:
    image: docker.io/test/router:latest
    tmp_dir:
      - name: prompt.txt
        path: ` + promptFile + `
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	bg := result["build_gate"].(map[string]any)
	router := bg["router"].(map[string]any)

	if _, hasTmpDir := router["tmp_dir"]; hasTmpDir {
		t.Errorf("expected 'tmp_dir' to be removed from router after bundle upload")
	}

	tmpBundle, hasTmpBundle := router["tmp_bundle"]
	if !hasTmpBundle {
		t.Fatalf("expected 'tmp_bundle' to be set in router after bundle upload")
	}

	bundle := tmpBundle.(map[string]any)
	if bundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("expected bundle_id=test-bundle-id, got %q", bundle["bundle_id"])
	}
	if bundle["cid"] != "bafytest" {
		t.Errorf("expected cid=bafytest, got %q", bundle["cid"])
	}
	if bundle["digest"] != "sha256:deadbeef" {
		t.Errorf("expected digest=sha256:deadbeef, got %q", bundle["digest"])
	}

	entries, ok := bundle["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries to be []any, got %T", bundle["entries"])
	}
	if len(entries) != 1 || entries[0] != "prompt.txt" {
		t.Errorf("expected entries=[prompt.txt], got %v", entries)
	}
}

// TestBuildSpecPayload_TmpDir_Healing verifies that tmp_dir entries in
// build_gate.healing.by_error_kind.* are archived, uploaded, and replaced with tmp_bundle.
func TestBuildSpecPayload_TmpDir_Healing(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	patchFile := filepath.Join(tmpDir, "patch.sh")
	patchContent := "#!/bin/sh\necho fix"
	if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
		t.Fatalf("write patch file: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      code:
        retries: 1
        image: docker.io/test/heal:latest
        tmp_dir:
          - name: patch.sh
            path: ` + patchFile + `
  router:
    image: docker.io/test/router:latest
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	bg := result["build_gate"].(map[string]any)
	healing := bg["healing"].(map[string]any)
	byErrorKind := healing["by_error_kind"].(map[string]any)
	code := byErrorKind["code"].(map[string]any)

	if _, hasTmpDir := code["tmp_dir"]; hasTmpDir {
		t.Errorf("expected 'tmp_dir' to be removed from healing action after bundle upload")
	}

	tmpBundle, hasTmpBundle := code["tmp_bundle"]
	if !hasTmpBundle {
		t.Fatalf("expected 'tmp_bundle' to be set in healing action after bundle upload")
	}

	bundle := tmpBundle.(map[string]any)
	if bundle["bundle_id"] != "test-bundle-id" {
		t.Errorf("expected bundle_id=test-bundle-id, got %q", bundle["bundle_id"])
	}
	if bundle["cid"] != "bafytest" {
		t.Errorf("expected cid=bafytest, got %q", bundle["cid"])
	}
	if bundle["digest"] != "sha256:deadbeef" {
		t.Errorf("expected digest=sha256:deadbeef, got %q", bundle["digest"])
	}

	entries, ok := bundle["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries to be []any, got %T", bundle["entries"])
	}
	if len(entries) != 1 || entries[0] != "patch.sh" {
		t.Errorf("expected entries=[patch.sh], got %v", entries)
	}
}

// TestBuildSpecPayload_TmpDir_InvalidPathType verifies a deterministic error when a
// tmp_dir entry has a non-string path value.
func TestBuildSpecPayload_TmpDir_InvalidPathType(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	specPath := filepath.Join(tmpDir, "spec.yaml")
	// YAML integer for path — should produce a typed error.
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: config.json
        path: 12345
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for non-string path in tmp_dir, got nil")
	}
	if !strings.Contains(err.Error(), "expected string path") {
		t.Errorf("expected 'expected string path' in error, got: %v", err)
	}
}

// TestBuildSpecPayload_TmpDir_MissingFile verifies a deterministic error when a
// tmp_dir path points to a nonexistent file.
func TestBuildSpecPayload_TmpDir_MissingFile(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: missing.txt
        path: /nonexistent/path/missing.txt
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for missing tmp_dir file, got nil")
	}
	// The error comes from stat or read failing
	if !strings.Contains(err.Error(), "stat") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected stat or file-not-found error, got: %v", err)
	}
}

// TestBuildSpecPayload_TmpDir_PathExpansion verifies that tmp_dir path entries
// support ~ home-dir expansion and $ENV_VAR substitution via resolvePath.
func TestBuildSpecPayload_TmpDir_PathExpansion(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "expand.txt")
	configContent := "expanded content"
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}

	// Build a path that uses ~ if the file is under the home dir, otherwise use $VAR expansion.
	var tildeFile string
	if strings.HasPrefix(tmpDir, home+"/") {
		rel := strings.TrimPrefix(configFile, home+"/")
		tildeFile = "~/" + rel
	} else {
		tildeFile = configFile
	}

	// Use an env-var path for the env-expansion sub-test.
	envVarName := "PLOY_TEST_TMPDIR_PATH_" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Setenv(envVarName, configFile)
	envFile := "$" + envVarName

	tests := []struct {
		name string
		path string
	}{
		{"tilde expansion", tildeFile},
		{"env expansion", envFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(tmpDir, "spec-"+tt.name+".yaml")
			specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: expand.txt
        path: ` + tt.path + `
`
			if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
				t.Fatalf("write spec file: %v", err)
			}

			payload, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
			if err != nil {
				t.Fatalf("buildSpecPayload error: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(payload, &result); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}

			steps := result["steps"].([]any)
			step0 := steps[0].(map[string]any)

			if _, hasTmpDir := step0["tmp_dir"]; hasTmpDir {
				t.Errorf("expected 'tmp_dir' to be removed after bundle upload")
			}

			if _, hasTmpBundle := step0["tmp_bundle"]; !hasTmpBundle {
				t.Errorf("expected 'tmp_bundle' to be set after bundle upload")
			}
		})
	}
}

// TestBuildSpecPayload_TmpDir_Mixed verifies mixed valid and invalid tmp_dir entries
// across steps, router, and healing blocks.
func TestBuildSpecPayload_TmpDir_Mixed(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	validFile := filepath.Join(tmpDir, "valid.txt")
	if err := os.WriteFile(validFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write valid file: %v", err)
	}

	tests := []struct {
		name        string
		specContent string
		wantErrSub  string
	}{
		{
			name: "steps: valid path then missing file",
			specContent: `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: valid.txt
        path: ` + validFile + `
      - name: missing.txt
        path: /nonexistent/missing.txt
`,
			// stat error for missing file
			wantErrSub: "stat",
		},
		{
			name: "steps: valid path then invalid path type",
			specContent: `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: valid.txt
        path: ` + validFile + `
      - name: bad.txt
        path: 999
`,
			wantErrSub: "expected string path",
		},
		{
			name: "router: valid then missing file",
			specContent: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/heal:latest
  router:
    image: docker.io/test/router:latest
    tmp_dir:
      - name: ok.txt
        path: ` + validFile + `
      - name: gone.txt
        path: /nonexistent/gone.txt
`,
			wantErrSub: "stat",
		},
		{
			name: "healing: missing file",
			specContent: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/heal:latest
        tmp_dir:
          - name: gone.txt
            path: /nonexistent/gone.txt
  router:
    image: docker.io/test/router:latest
`,
			wantErrSub: "stat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(tmpDir, "spec-"+strings.ReplaceAll(tt.name, " ", "-")+".yaml")
			if err := os.WriteFile(specPath, []byte(tt.specContent), 0o644); err != nil {
				t.Fatalf("write spec file: %v", err)
			}
			_, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected %q in error, got: %v", tt.wantErrSub, err)
			}
		})
	}
}

// TestBuildSpecPayload_TmpDir_NilBaseNoTmpDir verifies that when there are no
// tmp_dir sections, buildSpecPayload succeeds even with nil base/client.
func TestBuildSpecPayload_TmpDir_NilBaseNoTmpDir(t *testing.T) {
	tmpDir := t.TempDir()

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// nil base/client is fine when no tmp_dir is present
	payload, err := buildSpecPayload(context.Background(), nil, nil, specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
}

// TestBuildSpecPayload_TmpDir_NilBaseWithTmpDir verifies that when tmp_dir sections
// are present but base is nil, buildSpecPayload returns a descriptive error.
func TestBuildSpecPayload_TmpDir_NilBaseWithTmpDir(t *testing.T) {
	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: config.json
        path: ` + configFile + `
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := buildSpecPayload(context.Background(), nil, nil, specPath, nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error when tmp_dir found but base is nil, got nil")
	}
	if !strings.Contains(err.Error(), "no server base URL") {
		t.Errorf("expected 'no server base URL' in error, got: %v", err)
	}
}
