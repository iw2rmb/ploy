package mods

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadInputTar_InvokesArtifactUploader(t *testing.T) {
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "input.tar")
	if err := os.WriteFile(tarPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("prepare tar: %v", err)
	}
	fake := &recordingUploader{}
	runner := &ModRunner{}
	runner.SetArtifactUploader(fake)

	err := runner.uploadInputTar(context.Background(), "http://filer:8888", "exec-123", tarPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.files) != 1 {
		t.Fatalf("expected one upload, got %d", len(fake.files))
	}
	req := fake.files[0]
	if req.base != "http://filer:8888" {
		t.Fatalf("base mismatch: %s", req.base)
	}
	if req.key != "mods/exec-123/input.tar" {
		t.Fatalf("key mismatch: %s", req.key)
	}
	if req.path != tarPath {
		t.Fatalf("path mismatch: %s", req.path)
	}
	if req.contentType != "application/octet-stream" {
		t.Fatalf("content-type mismatch: %s", req.contentType)
	}
}

func TestSubstituteORWTemplateVars_ReplacesValuesWithoutEnv(t *testing.T) {
	tmp := t.TempDir()
	pre := filepath.Join(tmp, "orw.pre.hcl")
	content := "env { MOD=${MOD_ID} DIFF=${DIFF_KEY} OUT=${OUT_HOST_DIR} API=${PLOY_API_URL} INPUT=${INPUT_URL} IMG=${ORW_IMAGE} DC=${NOMAD_DC} }\n"
	if err := os.WriteFile(pre, []byte(content), 0644); err != nil {
		t.Fatalf("write pre: %v", err)
	}
	vars := map[string]string{
		"MODS_CONTEXT_DIR":     tmp,
		"MODS_OUT_DIR":         filepath.Join(tmp, "out"),
		"PLOY_CONTROLLER":      "https://api.dev.ployman.app/v1",
		"MOD_ID":               "mod-e-1",
		"PLOY_SEAWEEDFS_URL":   "http://filer:8888",
		"MODS_DIFF_KEY":        "mods/mod-e-1/branches/b-1/steps/s-1/diff.patch",
		"MODS_ORW_APPLY_IMAGE": "registry.dev.ployman.app/openrewrite-jvm:latest",
		"NOMAD_DC":             "dc9",
	}
	out, err := substituteORWTemplateVars(pre, "run-1", vars)
	if err != nil {
		t.Fatalf("subst err: %v", err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	// Verify key replacements occurred and /v1 suffix was trimmed in API base
	if want := "MOD=mod-e-1"; !contains(s, want) {
		t.Fatalf("missing %s in %s", want, s)
	}
	if want := "DIFF=mods/mod-e-1/branches/b-1/steps/s-1/diff.patch"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "OUT=" + vars["MODS_OUT_DIR"]; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "API=https://api.dev.ployman.app"; !contains(s, want) {
		t.Fatalf("missing %s", want)
	}
	if want := "INPUT=http://filer:8888/artifacts/mods/mod-e-1/input.tar"; !contains(s, want) {
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

type recordingUploader struct {
	files []struct{ base, key, path, contentType string }
}

func (r *recordingUploader) UploadFile(ctx context.Context, baseURL, key, srcPath, contentType string) error {
	r.files = append(r.files, struct{ base, key, path, contentType string }{
		base:        baseURL,
		key:         key,
		path:        srcPath,
		contentType: contentType,
	})
	return nil
}

func (r *recordingUploader) UploadJSON(ctx context.Context, baseURL, key string, body []byte) error {
	r.files = append(r.files, struct{ base, key, path, contentType string }{
		base: baseURL,
		key:  key,
	})
	return nil
}
