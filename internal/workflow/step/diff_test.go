package step

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountPatchStats(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  PatchStats
	}{
		{
			name:  "empty patch",
			patch: "",
			want:  PatchStats{},
		},
		{
			name: "single file change",
			patch: `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 package main
-var x = 1
+var x = 2
`,
			want: PatchStats{FilesChanged: 1, LinesAdded: 1, LinesRemoved: 1},
		},
		{
			name: "two files",
			patch: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 line1
+line2
+line3
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,2 +1,1 @@
 keep
-remove
`,
			want: PatchStats{FilesChanged: 2, LinesAdded: 2, LinesRemoved: 1},
		},
		{
			name: "no-index diff",
			patch: `diff --no-index a/x.txt b/x.txt
--- a/x.txt
+++ b/x.txt
@@ -1 +1 @@
-old
+new
`,
			want: PatchStats{FilesChanged: 1, LinesAdded: 1, LinesRemoved: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountPatchStats([]byte(tt.patch))
			if got != tt.want {
				t.Errorf("CountPatchStats() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFilesystemDiffGenerator_Generate(t *testing.T) {
	// Create a temporary directory for the test workspace.
	tmpDir := t.TempDir()

	// Initialize a git repository.
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for the test.
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create an initial file and commit.
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", "test.txt").Run(); err != nil {
		t.Fatalf("failed to add test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Migify the file to create a diff.
	if err := os.WriteFile(testFile, []byte("modified content\n"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	// Create diff generator and generate diff.
	generator := NewFilesystemDiffGenerator()
	ctx := context.Background()

	diffBytes, err := generator.Generate(ctx, tmpDir)
	if err != nil {
		t.Fatalf("failed to generate diff: %v", err)
	}

	// Verify diff contains expected content.
	diffStr := string(diffBytes)
	if !strings.Contains(diffStr, "test.txt") {
		t.Errorf("diff should contain file name 'test.txt', got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "-initial content") {
		t.Errorf("diff should contain old content '-initial content', got: %s", diffStr)
	}
	if !strings.Contains(diffStr, "+modified content") {
		t.Errorf("diff should contain new content '+modified content', got: %s", diffStr)
	}
}

func TestFilesystemDiffGenerator_Generate_NoDiff(t *testing.T) {
	// Create a temporary directory for the test workspace.
	tmpDir := t.TempDir()

	// Initialize a git repository.
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for the test.
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create an initial file and commit.
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", "test.txt").Run(); err != nil {
		t.Fatalf("failed to add test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create diff generator and generate diff (no changes).
	generator := NewFilesystemDiffGenerator()
	ctx := context.Background()

	diffBytes, err := generator.Generate(ctx, tmpDir)
	if err != nil {
		t.Fatalf("failed to generate diff: %v", err)
	}

	// Verify diff is empty.
	if len(diffBytes) > 0 {
		t.Errorf("expected empty diff, got: %s", string(diffBytes))
	}
}

func TestFilesystemDiffGenerator_Generate_NonGitRepo(t *testing.T) {
	// Create a temporary directory that is NOT a git repository.
	tmpDir := t.TempDir()

	// Create diff generator and attempt to generate diff.
	generator := NewFilesystemDiffGenerator()
	ctx := context.Background()

	_, err := generator.Generate(ctx, tmpDir)
	if err == nil {
		t.Error("expected error for non-git repository, got nil")
	}
}

func TestFilesystemDiffGenerator_Generate_ContextCancellation(t *testing.T) {
	// Create a temporary directory for the test workspace.
	tmpDir := t.TempDir()

	// Initialize a git repository.
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for the test.
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create an initial file and commit.
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", "test.txt").Run(); err != nil {
		t.Fatalf("failed to add test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create diff generator with cancelled context.
	generator := NewFilesystemDiffGenerator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := generator.Generate(ctx, tmpDir)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
