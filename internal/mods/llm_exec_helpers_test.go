package mods

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLLMPrepareContextCopiesSourcesForWorkspacePaths(t *testing.T) {
	baseDir := t.TempDir()
	repoRoot := t.TempDir()

	rel := filepath.Join("src", "healing", "java", "e2e", "FailHealing.java")
	abs := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("class FailHealing {\n  void f() { UnknownClass x; }\n}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	branch := BranchSpec{
		ID:   "llm-branch",
		Type: string(StepTypeLLMExec),
		Inputs: map[string]interface{}{
			"build_error": "[ERROR] /workspace/src/healing/java/e2e/FailHealing.java:[4,9] cannot find symbol",
		},
	}

	ctxDir, err := llmPrepareContext(baseDir, branch, repoRoot, nil, context.Background())
	if err != nil {
		t.Fatalf("llmPrepareContext: %v", err)
	}

	copied := filepath.Join(ctxDir, "sources", rel)
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected source copy at %s: %v", copied, err)
	}
}
