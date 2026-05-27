package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCommit_IgnoresRootTargetDirectory(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepoWithSingleCommit(t)
	ctx := context.Background()
	_ = prepareRepoWithTargetIgnore(t, ctx, repoDir)

	if err := os.MkdirAll(filepath.Join(repoDir, "target"), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "target", "generated.txt"), []byte("generated\n"), 0o644); err != nil {
		t.Fatalf("write target/generated.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}

	created, err := EnsureCommit(ctx, repoDir, "Test User", "test@example.com", "mutate main")
	if err != nil {
		t.Fatalf("EnsureCommit() error = %v", err)
	}
	if !created {
		t.Fatalf("EnsureCommit() created = false, want true")
	}
}

func TestEnsureCommit_SkipsRepositoryHooks(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepoWithSingleCommit(t)
	ctx := context.Background()
	if err := runGitCommand(ctx, repoDir, nil, "config", "--unset", "core.hooksPath"); err != nil {
		t.Fatalf("git config --unset core.hooksPath: %v", err)
	}

	hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
	hookRanPath := filepath.Join(repoDir, ".git", "pre-commit-ran")
	hook := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf ran > .git/pre-commit-ran\n" +
		"exit 1\n"
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatalf("write pre-commit hook: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}

	created, err := EnsureCommit(ctx, repoDir, "Test User", "test@example.com", "mutate main")
	if err != nil {
		t.Fatalf("EnsureCommit() error = %v", err)
	}
	if !created {
		t.Fatalf("EnsureCommit() created = false, want true")
	}
	if _, err := os.Stat(hookRanPath); !os.IsNotExist(err) {
		t.Fatalf("pre-commit hook ran, stat err = %v", err)
	}
}
