package mods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadInputTar_InvokesPutFileWithExpectedKey(t *testing.T) {
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "input.tar")
	if err := os.WriteFile(tarPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("prepare tar: %v", err)
	}
	called := false
	var gotBase, gotKey, gotCT, gotPath string
	prev := putFileFn
	putFileFn = func(seaweedBase, key, srcPath, contentType string) error {
		called = true
		gotBase, gotKey, gotPath, gotCT = seaweedBase, key, srcPath, contentType
		return nil
	}
	defer func() { putFileFn = prev }()

	err := uploadInputTar("http://filer:8888", "exec-123", tarPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected putFileFn to be called")
	}
	if gotBase != "http://filer:8888" {
		t.Fatalf("base mismatch: %s", gotBase)
	}
	if gotKey != "mods/exec-123/input.tar" {
		t.Fatalf("key mismatch: %s", gotKey)
	}
	if gotPath != tarPath {
		t.Fatalf("path mismatch: %s", gotPath)
	}
	if gotCT != "application/octet-stream" {
		t.Fatalf("content-type mismatch: %s", gotCT)
	}
}

func TestSubstituteORWTemplateVars_ReplacesValuesWithoutEnv(t *testing.T) {
	tmp := t.TempDir()
	pre := filepath.Join(tmp, "orw.pre.hcl")
	content := "env { EXEC=${EXECUTION_ID} DIFF=${DIFF_KEY} OUT=${OUT_HOST_DIR} API=${PLOY_API_URL} INPUT=${INPUT_URL} IMG=${ORW_IMAGE} DC=${NOMAD_DC} }\n"
	if err := os.WriteFile(pre, []byte(content), 0644); err != nil {
		t.Fatalf("write pre: %v", err)
	}
	vars := map[string]string{
		"MODS_CONTEXT_DIR":       tmp,
		"MODS_OUT_DIR":           filepath.Join(tmp, "out"),
		"PLOY_CONTROLLER":        "https://api.dev.ployman.app/v1",
		"PLOY_MODS_EXECUTION_ID": "e-1",
		"PLOY_SEAWEEDFS_URL":     "http://filer:8888",
		"MODS_DIFF_KEY":          "mods/e-1/branches/b-1/steps/s-1/diff.patch",
		"MODS_ORW_APPLY_IMAGE":   "registry.dev.ployman.app/openrewrite-jvm:latest",
		"NOMAD_DC":               "dc9",
	}
	out, err := substituteORWTemplateVars(pre, "run-1", vars)
	if err != nil {
		t.Fatalf("subst err: %v", err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	// Verify key replacements occurred and /v1 suffix was trimmed in API base
	if want := "EXEC=e-1"; !contains(s, want) {
		t.Fatalf("missing %s in %s", want, s)
	}
	if want := "DIFF=mods/e-1/branches/b-1/steps/s-1/diff.patch"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "OUT=" + vars["MODS_OUT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "API=https://api.dev.ployman.app"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "INPUT=http://filer:8888/artifacts/mods/e-1/input.tar"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "IMG=" + vars["MODS_ORW_APPLY_IMAGE"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "DC=dc9"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && (s[0:len(sub)] == sub || contains(s[1:], sub))))
}
