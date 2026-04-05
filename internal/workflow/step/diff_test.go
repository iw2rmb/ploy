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

// initGitRepo creates a git repo in a temp dir with an initial committed file.
// Returns the temp dir path and the path to the test file.
func initGitRepo(t *testing.T, content string) (dir, filePath string) {
	t.Helper()
	dir = t.TempDir()

	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@example.com"},
		{"-C", dir, "config", "user.name", "Test User"},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	filePath = filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	for _, args := range [][]string{
		{"-C", dir, "add", "test.txt"},
		{"-C", dir, "commit", "-m", "Initial commit"},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return dir, filePath
}

func TestFilesystemDiffGenerator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func(t *testing.T) string // returns workspace path
		wantErr      bool
		wantEmpty    bool
		wantContains []string
	}{
		{
			name: "generates diff for modified file",
			setup: func(t *testing.T) string {
				dir, filePath := initGitRepo(t, "initial content\n")
				if err := os.WriteFile(filePath, []byte("modified content\n"), 0644); err != nil {
					t.Fatalf("modify file: %v", err)
				}
				return dir
			},
			wantContains: []string{"test.txt", "-initial content", "+modified content"},
		},
		{
			name: "empty diff when no changes",
			setup: func(t *testing.T) string {
				dir, _ := initGitRepo(t, "content\n")
				return dir
			},
			wantEmpty: true,
		},
		{
			name: "error for non-git repo",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: true,
		},
		{
			name: "error on cancelled context",
			setup: func(t *testing.T) string {
				dir, _ := initGitRepo(t, "content\n")
				return dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workspace := tt.setup(t)
			generator := NewFilesystemDiffGenerator()

			ctx := context.Background()
			if tt.name == "error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			diffBytes, err := generator.Generate(ctx, workspace)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantEmpty {
				if len(diffBytes) > 0 {
					t.Errorf("expected empty diff, got: %s", string(diffBytes))
				}
				return
			}
			diffStr := string(diffBytes)
			for _, s := range tt.wantContains {
				if !strings.Contains(diffStr, s) {
					t.Errorf("diff missing %q, got: %s", s, diffStr)
				}
			}
		})
	}
}
