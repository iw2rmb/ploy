package mods

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLLMEntrypointGeneratesPatchFromFirstErrorHint(t *testing.T) {
	// Skip on Windows due to shell assumptions
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	wd, _ := os.Getwd()
	// Repo root is two dirs up from internal/mods
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	entry := filepath.Join(repoRoot, "services", "langgraph-runner", "entrypoint.sh")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("entrypoint script not found: %v", err)
	}

	// Prepare temp dirs
	outDir := t.TempDir()
	ctxDir := t.TempDir()
	// Prepare inputs.json with top-level first_error_file/line and a sources snapshot
	rel := "src/healing/java/e2e/FailHealing.java"
	srcPath := filepath.Join(ctxDir, "sources", rel)
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a small file with at least 4 lines; line 4 should be modified
	content := "package e2e;\npublic class FailHealing {\n  void x(){}\n  UnknownClass y;\n}\n"
	if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	inputs := `{
  "language": "java",
  "lane": "C",
  "last_error": {"stdout": "", "stderr": "cannot find symbol"},
  "first_error_file": "` + rel + `",
  "first_error_line": 4,
  "errors": []
}`
	if err := os.WriteFile(filepath.Join(ctxDir, "inputs.json"), []byte(inputs), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", entry)
	cmd.Env = append(os.Environ(),
		"RUN_ID=llm-exec-llm-1-12345",
		"OUTPUT_DIR="+outDir,
		"CONTEXT_DIR="+ctxDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("entrypoint error: %v\n%s", err, string(out))
	}
	// Verify diff.patch exists and mentions our file
	diff := filepath.Join(outDir, "diff.patch")
	b, err := os.ReadFile(diff)
	if err != nil {
		t.Fatalf("diff not found: %v\nstdout: %s", err, string(out))
	}
	s := string(b)
	if !strings.Contains(s, "a/"+rel) || !strings.Contains(s, "b/"+rel) {
		t.Fatalf("diff does not reference expected file: %s\n%s", rel, s)
	}
	// Ensure line edit comment appears
	if !strings.Contains(s, "+//") || !strings.Contains(s, "UnknownClass") {
		t.Fatalf("expected commented offending line in diff (has +// and UnknownClass)\n%s", s)
	}
}
