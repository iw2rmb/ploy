package prep

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCodexRunnerRun(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("prep prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	gitPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(gitPath, []byte(`#!/bin/sh
if [ "$1" = "clone" ]; then
  for last; do :; done
  mkdir -p "$last"
  exit 0
fi
exit 1
`), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte(`#!/bin/sh
echo "runner log"
cat <<JSON
{"schema_version":1,"repo_id":"$PLOY_PREP_REPO_ID","runner_mode":"simple","targets":{"build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]},"tactics_used":["go_default"],"attempts":[],"evidence":{"log_refs":["inline://prep/test"],"diagnostics":[]},"repro_check":{"status":"passed","details":"ok"},"prompt_delta_suggestion":{"status":"none","summary":"","candidate_lines":[]}}
JSON
`), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	runner := NewCodexRunner(CodexRunnerOptions{
		Command:    []string{codexPath},
		GitBinary:  gitPath,
		PromptPath: promptPath,
	})

	repoID := domaintypes.NewMigRepoID()
	result, err := runner.Run(context.Background(), RunRequest{
		Repo: store.MigRepo{
			ID:           repoID,
			PrepAttempts: 1,
			RepoUrl:      "https://example.com/repo.git",
			BaseRef:      "main",
			TargetRef:    "main",
		},
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := validateProfileJSON(result.ProfileJSON); err != nil {
		t.Fatalf("Run() profile validation error = %v", err)
	}
	if len(result.ResultJSON) == 0 {
		t.Fatal("Run() expected non-empty ResultJSON")
	}
}

func TestCodexRunnerRunCommandNotFound(t *testing.T) {
	t.Parallel()

	binDir := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptPath, []byte("prep prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	gitPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(gitPath, []byte(`#!/bin/sh
for last; do :; done
mkdir -p "$last"
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	runner := NewCodexRunner(CodexRunnerOptions{
		Command:    []string{"binary-that-does-not-exist"},
		GitBinary:  gitPath,
		PromptPath: promptPath,
	})

	_, err := runner.Run(context.Background(), RunRequest{
		Repo: store.MigRepo{
			ID:           domaintypes.NewMigRepoID(),
			PrepAttempts: 1,
			RepoUrl:      "https://example.com/repo.git",
			BaseRef:      "main",
			TargetRef:    "main",
		},
		Attempt: 1,
	})
	if err == nil {
		t.Fatal("Run() expected error")
	}

	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("Run() error type = %T, want *RunError", err)
	}
	if runErr.FailureCode != FailureCodeCommandNotFound {
		t.Fatalf("Run() failure code = %q, want %q", runErr.FailureCode, FailureCodeCommandNotFound)
	}
}
