package mods

import (
	"context"
	"encoding/json"
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

func TestLLMPrepareContextFallsBackToBuilderLogs(t *testing.T) {
	baseDir := t.TempDir()
	repoRoot := t.TempDir()

	// Seed repo with file expected by builder logs
	rel := filepath.Join("src", "main", "java", "e2e", "FailMissingSymbol.java")
	abs := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("package e2e;\nclass FailMissingSymbol { void run() { UnknownClass x; } }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldDL := downloadToFileFn
	defer func() { downloadToFileFn = oldDL }()

	const builderLog = `#11 6.891 [ERROR] COMPILATION ERROR : 
#11 6.891 [ERROR] /src/src/main/java/e2e/FailMissingSymbol.java:[6,9] cannot find symbol
#11 6.891 [ERROR] /src/src/main/java/e2e/FailMissingSymbol.java:[6,32] cannot find symbol
`

	var gotURL string
	downloadToFileFn = func(url, dest string) error {
		gotURL = url
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte(builderLog), 0o644)
	}

	const seaweedURL = "http://seaweed.test"
	if err := os.Setenv("PLOY_SEAWEEDFS_URL", seaweedURL); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer func() { _ = os.Unsetenv("PLOY_SEAWEEDFS_URL") }()

	branch := BranchSpec{
		ID:   "llm-branch",
		Type: string(StepTypeLLMExec),
		Inputs: map[string]interface{}{
			"build_error":      "internal error",
			"builder_logs_key": "build-logs/mod-build.log",
		},
	}

	ctxDir, err := llmPrepareContext(baseDir, branch, repoRoot, nil, context.Background())
	if err != nil {
		t.Fatalf("llmPrepareContext: %v", err)
	}

	if gotURL != seaweedURL+"/artifacts/build-logs/mod-build.log" {
		t.Fatalf("unexpected builder log url: %s", gotURL)
	}

	data, err := os.ReadFile(filepath.Join(ctxDir, "inputs.json"))
	if err != nil {
		t.Fatalf("read inputs: %v", err)
	}

	var payload struct {
		FirstErrorFile string `json:"first_error_file"`
		FirstErrorLine int    `json:"first_error_line"`
		BuilderLogsKey string `json:"builder_logs_key"`
		Errors         []struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal inputs.json: %v", err)
	}

	if payload.BuilderLogsKey != "build-logs/mod-build.log" {
		t.Fatalf("expected builder logs key propagated, got %q", payload.BuilderLogsKey)
	}
	if payload.FirstErrorFile != "src/main/java/e2e/FailMissingSymbol.java" {
		t.Fatalf("unexpected first_error_file: %q", payload.FirstErrorFile)
	}
	if payload.FirstErrorLine != 6 {
		t.Fatalf("unexpected first_error_line: %d", payload.FirstErrorLine)
	}
	if len(payload.Errors) == 0 || payload.Errors[0].File != "src/main/java/e2e/FailMissingSymbol.java" {
		t.Fatalf("expected parsed errors from builder logs, got %+v", payload.Errors)
	}

	copied := filepath.Join(ctxDir, "sources", rel)
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("expected source copy at %s: %v", copied, err)
	}
}
