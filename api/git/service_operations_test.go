package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitChangesCreatesCommitWithDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, repo, "init")

	svc := NewService(ServiceConfig{})
	file := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := svc.CommitChanges(context.Background(), repo, "initial commit"); err != nil {
		t.Fatalf("CommitChanges error: %v", err)
	}

	hash, err := svc.GetCommitHash(context.Background(), repo)
	if err != nil || hash == "" {
		t.Fatalf("expected commit hash, got %q (err=%v)", hash, err)
	}

	author := runGitOutput(t, repo, "log", "-1", "--pretty=%an")
	if author != "Ploy Transflow" {
		t.Fatalf("expected author to be set by service, got %q", author)
	}

	if err := svc.CommitChanges(context.Background(), repo, "no changes"); err != nil {
		t.Fatalf("expected no error when nothing to commit, got %v", err)
	}
}

func TestBranchOperations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initGitRepo(t)
	svc := NewService(ServiceConfig{})

	if err := svc.CreateBranch(context.Background(), repo, "feature"); err != nil {
		t.Fatalf("CreateBranch error: %v", err)
	}

	branches := runGitOutput(t, repo, "branch", "--list", "feature")
	if strings.TrimSpace(branches) == "" {
		t.Fatalf("expected feature branch to exist")
	}

	if err := svc.CreateBranchAndCheckout(context.Background(), repo, "feature"); err != nil {
		t.Fatalf("CreateBranchAndCheckout existing branch: %v", err)
	}
	current := runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	if current != "feature" {
		t.Fatalf("expected to be on feature branch, got %q", current)
	}

	if err := svc.CreateBranchAndCheckout(context.Background(), repo, "newbranch"); err != nil {
		t.Fatalf("CreateBranchAndCheckout new branch: %v", err)
	}
	current = runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	if current != "newbranch" {
		t.Fatalf("expected to be on newbranch, got %q", current)
	}
}

func TestResetToCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initGitRepo(t)
	svc := NewService(ServiceConfig{})

	file := filepath.Join(repo, "initial.txt")
	if err := os.WriteFile(file, []byte("change\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repo, "add", "initial.txt")
	runGit(t, repo, "commit", "-m", "second commit")

	first := runGitOutput(t, repo, "rev-list", "--max-parents=0", "HEAD")
	if err := svc.ResetToCommit(context.Background(), repo, first); err != nil {
		t.Fatalf("ResetToCommit error: %v", err)
	}

	head := runGitOutput(t, repo, "rev-parse", "HEAD")
	if head != first {
		t.Fatalf("expected HEAD to match first commit, got %q", head)
	}
}

func TestRepositoryStatsHelpers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initGitRepo(t)
	svc := NewService(ServiceConfig{})

	// create staged new file with two lines
	staged := filepath.Join(repo, "staged.txt")
	if err := os.WriteFile(staged, []byte("uno\ndos\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	runGit(t, repo, "add", "staged.txt")

	// modify tracked file without staging
	tracked := filepath.Join(repo, "initial.txt")
	if err := os.WriteFile(tracked, []byte("original\nextra\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}

	count, err := svc.CountChangedFiles(context.Background(), repo)
	if err != nil {
		t.Fatalf("CountChangedFiles error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 changed files (1 staged, 1 unstaged), got %d", count)
	}

	added, removed, err := svc.GetLineChanges(context.Background(), repo)
	if err != nil {
		t.Fatalf("GetLineChanges error: %v", err)
	}
	if added < 3 || removed != 0 {
		t.Fatalf("expected line additions to include staged and unstaged changes, got added=%d removed=%d", added, removed)
	}

	if err := svc.CommitChanges(context.Background(), repo, "capture changes"); err != nil {
		t.Fatalf("commit changes: %v", err)
	}

	history, err := svc.GetFileHistory(context.Background(), repo, []string{"initial.txt", "staged.txt", "missing.txt"})
	if err != nil {
		t.Fatalf("GetFileHistory error: %v", err)
	}
	if len(history["initial.txt"]) < 2 {
		t.Fatalf("expected history for initial.txt to include multiple commits, got %d", len(history["initial.txt"]))
	}
	if len(history["staged.txt"]) != 1 {
		t.Fatalf("expected staged.txt to have one commit entry")
	}
	if logs := history["missing.txt"]; len(logs) != 0 {
		t.Fatalf("expected missing file to have empty history, got %v", logs)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := execCommand(dir, args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
