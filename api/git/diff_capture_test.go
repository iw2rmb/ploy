package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetDiffIncludesLineCounts validates diff captures include per-file metrics.
func TestGetDiffIncludesLineCounts(t *testing.T) {
	repo := initGitRepo(t)

	modified := filepath.Join(repo, "initial.txt")
	if err := os.WriteFile(modified, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

	added := filepath.Join(repo, "added.txt")
	if err := os.WriteFile(added, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write added file: %v", err)
	}

	svc := NewService(ServiceConfig{})
	diffs, err := svc.GetDiff(context.Background(), repo)
	if err != nil {
		t.Fatalf("GetDiff error: %v", err)
	}

	if len(diffs) == 0 {
		t.Fatalf("expected diffs, got none")
	}

	foundModified := false
	foundAdded := false
	for _, diff := range diffs {
		switch diff.File {
		case "initial.txt":
			foundModified = true
			if diff.Type != "modified" {
				t.Fatalf("expected modified type, got %q", diff.Type)
			}
			if diff.LinesAdded == 0 || diff.LinesRemoved == 0 {
				t.Fatalf("expected non-zero line counts for modified file, got added=%d removed=%d", diff.LinesAdded, diff.LinesRemoved)
			}
			if !strings.Contains(diff.UnifiedDiff, "alpha") {
				t.Fatalf("expected diff to include updated content, got %q", diff.UnifiedDiff)
			}
		case "added.txt":
			foundAdded = true
			if diff.Type != "added" {
				t.Fatalf("expected added type, got %q", diff.Type)
			}
			if diff.LinesAdded == 0 {
				t.Fatalf("expected positive LinesAdded for new file, got 0")
			}
		}
	}

	if !foundModified {
		t.Fatalf("expected diff for modified file")
	}
	if !foundAdded {
		t.Fatalf("expected diff for added file")
	}
}

// initGitRepo creates a temporary repository seeded with one commit for testing.
func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "initial.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	runGit(t, repo, "add", "initial.txt")
	runGit(t, repo, "commit", "-m", "initial commit")
	return repo
}

// runGit executes a git command in the provided directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := execCommand(dir, args...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// execCommand builds a git command scoped to the supplied directory.
func execCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}
