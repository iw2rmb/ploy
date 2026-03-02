package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeRepoSHAV1_UnchangedWorkspaceReturnsInputSHA(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepoWithSingleCommit(t)
	ctx := context.Background()
	repoSHAIn := gitStdout(t, repoDir, "rev-parse", "HEAD")

	repoSHAOut, err := ComputeRepoSHAV1(ctx, repoDir, repoSHAIn)
	if err != nil {
		t.Fatalf("ComputeRepoSHAV1() error = %v", err)
	}
	if repoSHAOut != repoSHAIn {
		t.Fatalf("repo_sha_out = %q, want %q", repoSHAOut, repoSHAIn)
	}

	headAfter := gitStdout(t, repoDir, "rev-parse", "HEAD")
	if headAfter != repoSHAIn {
		t.Fatalf("HEAD changed: got %q want %q", headAfter, repoSHAIn)
	}
}

func TestComputeRepoSHAV1_DeterministicForSameWorkspace(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepoWithSingleCommit(t)
	ctx := context.Background()
	repoSHAIn := gitStdout(t, repoDir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	out1, err := ComputeRepoSHAV1(ctx, repoDir, repoSHAIn)
	if err != nil {
		t.Fatalf("ComputeRepoSHAV1(first) error = %v", err)
	}
	out2, err := ComputeRepoSHAV1(ctx, repoDir, repoSHAIn)
	if err != nil {
		t.Fatalf("ComputeRepoSHAV1(second) error = %v", err)
	}

	if out1 == repoSHAIn {
		t.Fatalf("expected changed workspace to produce different sha, got same %q", out1)
	}
	if out1 != out2 {
		t.Fatalf("expected deterministic sha, got %q vs %q", out1, out2)
	}

	headAfter := gitStdout(t, repoDir, "rev-parse", "HEAD")
	if headAfter != repoSHAIn {
		t.Fatalf("HEAD changed: got %q want %q", headAfter, repoSHAIn)
	}
}

func initTestRepoWithSingleCommit(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	ctx := context.Background()

	if err := runGitCommand(ctx, repoDir, nil, "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return repoDir
}

func gitStdout(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	out, err := runGitOutput(context.Background(), repoDir, nil, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}
