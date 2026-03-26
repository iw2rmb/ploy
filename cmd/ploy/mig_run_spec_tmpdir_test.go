package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// TestBuildSpecPayload_TmpDir_NameNormalization verifies that tmp_dir[].name values are
// normalized with canonical filename rules before archiving and metadata emission.
func TestBuildSpecPayload_TmpDir_NameNormalization(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// Names with surrounding whitespace should be normalized (trimmed) before use.
	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: "  config.json  "
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
	bundle := step0["tmp_bundle"].(map[string]any)
	entries := bundle["entries"].([]any)
	if len(entries) != 1 || entries[0] != "config.json" {
		t.Errorf("expected normalized entries=[config.json], got %v", entries)
	}
}

// TestBuildSpecPayload_TmpDir_InvalidName verifies that invalid tmp_dir[].name values
// are rejected before archiving with a descriptive error.
func TestBuildSpecPayload_TmpDir_InvalidName(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	tests := []struct {
		name       string
		entryName  string
		wantErrSub string
	}{
		{
			name:       "path separator in name",
			entryName:  "sub/config.json",
			wantErrSub: "plain filename",
		},
		{
			name:       "dotdot name",
			entryName:  "..",
			wantErrSub: "plain filename",
		},
		{
			name:       "empty name",
			entryName:  "   ",
			wantErrSub: "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(tmpDir, "spec-"+strings.ReplaceAll(tt.name, " ", "-")+".yaml")
			specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: "` + tt.entryName + `"
        path: ` + configFile + `
`
			if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
				t.Fatalf("write spec file: %v", err)
			}
			_, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
			if err == nil {
				t.Fatal("expected error for invalid name, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected %q in error, got: %v", tt.wantErrSub, err)
			}
		})
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

// TestBuildSpecPayload_TmpDir_SymlinkFile verifies that a top-level tmp_dir path that
// is a symlink to a regular file is followed and its content ends up in the bundle.
func TestBuildSpecPayload_TmpDir_SymlinkFile(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("symlinked content"), 0o644); err != nil {
		t.Fatalf("write real file: %v", err)
	}
	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: link.txt
        path: ` + linkFile + `
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
	tmpBundle, hasTmpBundle := step0["tmp_bundle"]
	if !hasTmpBundle {
		t.Fatalf("expected 'tmp_bundle' to be set after bundle upload")
	}
	entries := tmpBundle.(map[string]any)["entries"].([]any)
	if len(entries) != 1 || entries[0] != "link.txt" {
		t.Errorf("expected entries=[link.txt], got %v", entries)
	}
}

// TestBuildSpecPayload_TmpDir_SymlinkDangling verifies that a top-level tmp_dir path
// that is a dangling symlink returns a deterministic error rather than being silently omitted.
func TestBuildSpecPayload_TmpDir_SymlinkDangling(t *testing.T) {
	srv, _ := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpDir := t.TempDir()

	linkFile := filepath.Join(tmpDir, "dangling.txt")
	if err := os.Symlink(filepath.Join(tmpDir, "nonexistent-target.txt"), linkFile); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	specPath := filepath.Join(tmpDir, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: dangling.txt
        path: ` + linkFile + `
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	_, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for dangling symlink, got nil")
	}
	if !strings.Contains(err.Error(), "stat") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected stat or file-not-found error, got: %v", err)
	}
}

// readTarEntries decompresses a gzip-compressed tar archive and returns the names
// of all entries (headers) in the order they appear.
func readTarEntries(t *testing.T, archiveBytes []byte) []string {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

// readTarFileContent returns the content of the named entry in a gzip-compressed tar.
func readTarFileContent(t *testing.T, archiveBytes []byte, entryName string) []byte {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Name == entryName {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read %s: %v", entryName, err)
			}
			return data
		}
	}
	t.Fatalf("entry %q not found in archive", entryName)
	return nil
}

// TestBuildSpecBundleArchive_DirectoryInput verifies that when a tmp_dir entry's path
// is a directory, the archive contains the directory header followed by all nested files
// in sorted order. File content must match the source.
func TestBuildSpecBundleArchive_DirectoryInput(t *testing.T) {
	base := t.TempDir()

	// Build: mydir/{a.txt, b.txt, sub/c.txt}
	mydir := filepath.Join(base, "mydir")
	if err := os.MkdirAll(filepath.Join(mydir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	files := map[string]string{
		filepath.Join(mydir, "a.txt"):       "alpha",
		filepath.Join(mydir, "b.txt"):       "beta",
		filepath.Join(mydir, "sub", "c.txt"): "gamma",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	entries := []tmpDirEntry{{Name: "mydir", Path: mydir}}
	archiveBytes, err := buildSpecBundleArchive(entries)
	if err != nil {
		t.Fatalf("buildSpecBundleArchive: %v", err)
	}

	got := readTarEntries(t, archiveBytes)
	want := []string{
		"mydir/",
		"mydir/a.txt",
		"mydir/b.txt",
		"mydir/sub/",
		"mydir/sub/c.txt",
	}
	if len(got) != len(want) {
		t.Fatalf("entry count: got %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("entry[%d]: got %q, want %q", i, got[i], name)
		}
	}

	// Verify file content.
	if content := readTarFileContent(t, archiveBytes, "mydir/a.txt"); string(content) != "alpha" {
		t.Errorf("mydir/a.txt content: got %q, want %q", content, "alpha")
	}
	if content := readTarFileContent(t, archiveBytes, "mydir/sub/c.txt"); string(content) != "gamma" {
		t.Errorf("mydir/sub/c.txt content: got %q, want %q", content, "gamma")
	}
}

// TestBuildSpecBundleArchive_RepeatedRunsDeterminism verifies that calling
// buildSpecBundleArchive twice with the same entries produces byte-identical archives.
func TestBuildSpecBundleArchive_RepeatedRunsDeterminism(t *testing.T) {
	base := t.TempDir()

	mydir := filepath.Join(base, "data")
	if err := os.MkdirAll(mydir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"x.txt", "y.txt"} {
		if err := os.WriteFile(filepath.Join(mydir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	plainFile := filepath.Join(base, "plain.txt")
	if err := os.WriteFile(plainFile, []byte("plain"), 0o644); err != nil {
		t.Fatalf("write plain.txt: %v", err)
	}

	entries := []tmpDirEntry{
		{Name: "data", Path: mydir},
		{Name: "plain.txt", Path: plainFile},
	}

	first, err := buildSpecBundleArchive(entries)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := buildSpecBundleArchive(entries)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Error("archive bytes differ between repeated runs with identical inputs")
	}
}

// TestBuildSpecBundleArchive_ShuffledInputDeterminism verifies that buildSpecBundleArchive
// produces byte-identical archives when entries are provided in different orderings,
// because entries are sorted by Name before archiving.
func TestBuildSpecBundleArchive_ShuffledInputDeterminism(t *testing.T) {
	base := t.TempDir()

	names := []string{"apple.txt", "cherry.txt", "banana.txt"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(base, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	orderings := [][]tmpDirEntry{
		{
			{Name: "apple.txt", Path: filepath.Join(base, "apple.txt")},
			{Name: "banana.txt", Path: filepath.Join(base, "banana.txt")},
			{Name: "cherry.txt", Path: filepath.Join(base, "cherry.txt")},
		},
		{
			{Name: "cherry.txt", Path: filepath.Join(base, "cherry.txt")},
			{Name: "apple.txt", Path: filepath.Join(base, "apple.txt")},
			{Name: "banana.txt", Path: filepath.Join(base, "banana.txt")},
		},
		{
			{Name: "banana.txt", Path: filepath.Join(base, "banana.txt")},
			{Name: "cherry.txt", Path: filepath.Join(base, "cherry.txt")},
			{Name: "apple.txt", Path: filepath.Join(base, "apple.txt")},
		},
	}

	reference, err := buildSpecBundleArchive(orderings[0])
	if err != nil {
		t.Fatalf("reference archive: %v", err)
	}

	for i, entries := range orderings[1:] {
		got, err := buildSpecBundleArchive(entries)
		if err != nil {
			t.Fatalf("ordering[%d]: %v", i+1, err)
		}
		if !bytes.Equal(reference, got) {
			t.Errorf("ordering[%d] produces different archive bytes than reference", i+1)
		}
	}
}

// TestBuildSpecPayload_TmpDir_DirectoryPath verifies that a tmp_dir entry whose path
// is a directory is archived and uploaded, and that the bundle entries list contains
// the directory name (not individual file paths).
func TestBuildSpecPayload_TmpDir_DirectoryPath(t *testing.T) {
	srv, lastUpload := newMockBundleServer(t)
	defer srv.Close()
	base := parsedBundleBase(t, srv)
	client := srv.Client()

	tmpRoot := t.TempDir()

	// Build: scripts/{run.sh, setup.sh}
	scriptsDir := filepath.Join(tmpRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	for _, name := range []string{"run.sh", "setup.sh"} {
		if err := os.WriteFile(filepath.Join(scriptsDir, name), []byte("#!/bin/sh"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	specPath := filepath.Join(tmpRoot, "spec.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    tmp_dir:
      - name: scripts
        path: ` + scriptsDir + `
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	payload, err := buildSpecPayload(context.Background(), base, client, specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}

	// Verify the archive sent to the server contains directory entries.
	archiveEntries := readTarEntries(t, *lastUpload)
	wantArchiveEntries := []string{"scripts/", "scripts/run.sh", "scripts/setup.sh"}
	if len(archiveEntries) != len(wantArchiveEntries) {
		t.Fatalf("archive entry count: got %v, want %v", archiveEntries, wantArchiveEntries)
	}
	for i, name := range wantArchiveEntries {
		if archiveEntries[i] != name {
			t.Errorf("archive entry[%d]: got %q, want %q", i, archiveEntries[i], name)
		}
	}

	// Verify the spec payload has tmp_bundle with the directory name as the single entry.
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
	entries := tmpBundle.(map[string]any)["entries"].([]any)
	if len(entries) != 1 || entries[0] != "scripts" {
		t.Errorf("expected entries=[scripts], got %v", entries)
	}
}
