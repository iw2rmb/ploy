package mods

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// run executes a command in the given directory and fails the test on error.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v: %s", name, args, err, string(out))
	}
}

// setupLocalRepos creates an origin repo with an initial commit on main and a bare clone.
func setupLocalRepos(t *testing.T, root string) (origin, bare string) {
	t.Helper()
	origin = filepath.Join(root, "origin")
	bare = filepath.Join(root, "bare.git")
	run(t, "", "git", "init", "-b", "main", origin)
	// Configure user for commits
	run(t, origin, "git", "config", "user.email", "test@example.com")
	run(t, origin, "git", "config", "user.name", "Test User")
	// Write file and commit
	_ = os.WriteFile(filepath.Join(origin, "README.md"), []byte("hello\n"), 0644)
	run(t, origin, "git", "add", ".")
	run(t, origin, "git", "commit", "-m", "chore: initial")
	// Create bare clone
	run(t, root, "git", "clone", "--bare", origin, bare)
	return origin, bare
}

// TestGitOperations_LocalCloneAndBranch verifies ARF Git operations on local repositories.
func TestGitOperations_LocalCloneAndBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	work := t.TempDir()
	_, bare := setupLocalRepos(t, work)

	// Use production Git ops (ARF-backed) against local repos
	gitOps := NewARFGitOperations(work)

	// Clone from bare into repo path
	clonePath := filepath.Join(work, "repo")
	if err := gitOps.CloneRepository(context.Background(), bare, "main", clonePath); err != nil {
		t.Fatalf("CloneRepository failed: %v", err)
	}

	// Create and checkout workflow branch
	branch := "workflow/local-test/1"
	if err := gitOps.CreateBranchAndCheckout(context.Background(), clonePath, branch); err != nil {
		t.Fatalf("CreateBranchAndCheckout failed: %v", err)
	}

	// Make a change and commit via GitOperations
	if err := os.WriteFile(filepath.Join(clonePath, "README.md"), []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gitOps.CommitChanges(context.Background(), clonePath, "feat: update readme"); err != nil {
		t.Fatalf("CommitChanges failed: %v", err)
	}
}

// TestGitOperations_PushToBare verifies push to a local bare repository.
func TestGitOperations_PushToBare(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	work := t.TempDir()
	_, bare := setupLocalRepos(t, work)

	gitOps := NewARFGitOperations(work)
	clonePath := filepath.Join(work, "repo2")
	if err := gitOps.CloneRepository(context.Background(), bare, "main", clonePath); err != nil {
		t.Fatalf("CloneRepository failed: %v", err)
	}
	branch := "workflow/local-test/2"
	if err := gitOps.CreateBranchAndCheckout(context.Background(), clonePath, branch); err != nil {
		t.Fatalf("CreateBranchAndCheckout failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clonePath, "NEW.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gitOps.CommitChanges(context.Background(), clonePath, "feat: add new file"); err != nil {
		t.Fatalf("CommitChanges failed: %v", err)
	}
	// Push back to the bare origin using its path as remote URL
	if err := gitOps.PushBranch(context.Background(), clonePath, bare, branch); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}
}
