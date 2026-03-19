package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodeTmpEntry round-trips a raw map[string]any tmp_dir entry through JSON to
// extract the typed Name and Content fields.
func decodeTmpEntry(t *testing.T, entry map[string]any) (name string, content []byte) {
	t.Helper()
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal tmp_dir entry: %v", err)
	}
	var te struct {
		Name    string `json:"name"`
		Content []byte `json:"content"`
	}
	if err := json.Unmarshal(entryJSON, &te); err != nil {
		t.Fatalf("unmarshal tmp_dir entry: %v", err)
	}
	return te.Name, te.Content
}

// TestBuildSpecPayload_TmpDir_Steps verifies that tmp_dir path entries in steps[]
// are resolved: file content is read and stored in the canonical content field.
func TestBuildSpecPayload_TmpDir_Steps(t *testing.T) {
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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	steps := result["steps"].([]any)
	step0 := steps[0].(map[string]any)
	tmpDirRaw := step0["tmp_dir"].([]any)
	entry := tmpDirRaw[0].(map[string]any)

	if _, hasPath := entry["path"]; hasPath {
		t.Errorf("expected 'path' to be removed from tmp_dir entry after resolution")
	}

	name, content := decodeTmpEntry(t, entry)
	if name != "config.json" {
		t.Errorf("expected name=config.json, got %q", name)
	}
	if string(content) != configContent {
		t.Errorf("expected content=%q, got %q", configContent, string(content))
	}
}

// TestBuildSpecPayload_TmpDir_Router verifies that tmp_dir path entries in build_gate.router
// are resolved to file content.
func TestBuildSpecPayload_TmpDir_Router(t *testing.T) {
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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	bg := result["build_gate"].(map[string]any)
	router := bg["router"].(map[string]any)
	tmpDirRaw := router["tmp_dir"].([]any)
	entry := tmpDirRaw[0].(map[string]any)

	if _, hasPath := entry["path"]; hasPath {
		t.Errorf("expected 'path' to be removed from router tmp_dir entry")
	}

	name, content := decodeTmpEntry(t, entry)
	if name != "prompt.txt" {
		t.Errorf("expected name=prompt.txt, got %q", name)
	}
	if string(content) != promptContent {
		t.Errorf("expected content=%q, got %q", promptContent, string(content))
	}
}

// TestBuildSpecPayload_TmpDir_Healing verifies that tmp_dir path entries in
// build_gate.healing.by_error_kind.* are resolved to file content.
func TestBuildSpecPayload_TmpDir_Healing(t *testing.T) {
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

	payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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
	tmpDirRaw := code["tmp_dir"].([]any)
	entry := tmpDirRaw[0].(map[string]any)

	if _, hasPath := entry["path"]; hasPath {
		t.Errorf("expected 'path' to be removed from healing tmp_dir entry")
	}

	name, content := decodeTmpEntry(t, entry)
	if name != "patch.sh" {
		t.Errorf("expected name=patch.sh, got %q", name)
	}
	if string(content) != patchContent {
		t.Errorf("expected content=%q, got %q", patchContent, string(content))
	}
}

// TestBuildSpecPayload_TmpDir_InvalidPathType verifies a deterministic error when a
// tmp_dir entry has a non-string path value.
func TestBuildSpecPayload_TmpDir_InvalidPathType(t *testing.T) {
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

	_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
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

	_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
	if err == nil {
		t.Fatal("expected error for missing tmp_dir file, got nil")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Errorf("expected 'read file' in error, got: %v", err)
	}
}

// TestBuildSpecPayload_TmpDir_Mixed verifies mixed valid and invalid tmp_dir entries
// across steps, router, and healing blocks.
func TestBuildSpecPayload_TmpDir_Mixed(t *testing.T) {
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
			wantErrSub: "read file",
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
			wantErrSub: "read file",
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
			wantErrSub: "read file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(tmpDir, "spec-"+strings.ReplaceAll(tt.name, " ", "-")+".yaml")
			if err := os.WriteFile(specPath, []byte(tt.specContent), 0o644); err != nil {
				t.Fatalf("write spec file: %v", err)
			}
			_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected %q in error, got: %v", tt.wantErrSub, err)
			}
		})
	}
}
