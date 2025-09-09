package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Test that generate-diff.sh produces a diff.patch file even when there are no changes.
func TestGenerateDiffProducesPatch(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	before := filepath.Join(baseDir, "before")
	after := filepath.Join(baseDir, "after")
	if err := os.MkdirAll(filepath.Join(before, "src"), 0o755); err != nil {
		t.Fatalf("mkdir before: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(after, "src"), 0o755); err != nil {
		t.Fatalf("mkdir after: %v", err)
	}
	// Create identical file in both trees
	content := []byte("class App {}\n")
	if err := os.WriteFile(filepath.Join(before, "src", "App.java"), content, 0o644); err != nil {
		t.Fatalf("write before: %v", err)
	}
	if err := os.WriteFile(filepath.Join(after, "src", "App.java"), content, 0o644); err != nil {
		t.Fatalf("write after: %v", err)
	}

	patchPath := filepath.Join(baseDir, "diff.patch")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	script := filepath.Join(repoRoot, "services", "openrewrite-jvm", "generate-diff.sh")
	cmd := exec.Command("bash", script, before, after, patchPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate-diff.sh failed: %v\nOutput:\n%s", err, string(out))
	}
	st, err := os.Stat(patchPath)
	if err != nil {
		t.Fatalf("diff.patch not found: %v\nOutput:\n%s", err, string(out))
	}
	if st.Size() < 0 { // should exist; size may be zero when no changes
		t.Fatalf("unexpected negative size")
	}
}
