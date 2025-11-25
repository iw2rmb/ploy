package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDecompressPatch verifies gzip decompression of patches.
func TestDecompressPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      []byte
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "empty patch",
			input:      []byte{},
			wantOutput: "",
			wantErr:    false,
		},
		{
			name:       "valid gzipped patch",
			input:      gzipBytes(t, []byte("diff --git a/file.txt b/file.txt")),
			wantOutput: "diff --git a/file.txt b/file.txt",
			wantErr:    false,
		},
		{
			name:    "invalid gzip data",
			input:   []byte("not gzip data"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decompressPatch(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decompressPatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.wantOutput {
				t.Errorf("decompressPatch() = %q, want %q", string(got), tt.wantOutput)
			}
		})
	}
}

// TestApplyGzippedPatch verifies patch application to a git workspace.
func TestApplyGzippedPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupRepo     func(t *testing.T, dir string)
		patch         string
		wantErr       bool
		validateAfter func(t *testing.T, dir string)
	}{
		{
			name: "empty patch applies cleanly",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			patch:   "",
			wantErr: false,
			validateAfter: func(t *testing.T, dir string) {
				// No changes expected.
				assertFileContent(t, filepath.Join(dir, "README.md"), "# Test\n")
			},
		},
		{
			name: "valid patch adds new file",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			patch: `diff --git a/newfile.txt b/newfile.txt
new file mode 100644
index 0000000..ce01362
--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1 @@
+hello
`,
			wantErr: false,
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "newfile.txt"), "hello\n")
			},
		},
		{
			name: "valid patch modifies existing file",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			patch: `diff --git a/README.md b/README.md
index 2a02d41..0527e6b 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-# Test
+# Test Modified
`,
			wantErr: false,
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "README.md"), "# Test Modified\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary workspace.
			workspace := t.TempDir()

			// Setup repository.
			tt.setupRepo(t, workspace)

			// Apply patch.
			ctx := context.Background()
			gzippedPatch := gzipBytes(t, []byte(tt.patch))
			err := applyGzippedPatch(ctx, workspace, gzippedPatch)

			if (err != nil) != tt.wantErr {
				t.Errorf("applyGzippedPatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateAfter != nil {
				tt.validateAfter(t, workspace)
			}
		})
	}
}

// TestRehydrateWorkspaceFromBaseAndDiffs verifies full rehydration flow.
func TestRehydrateWorkspaceFromBaseAndDiffs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupBase     func(t *testing.T, baseDir string)
		diffs         []string
		wantErr       bool
		validateAfter func(t *testing.T, destDir string)
	}{
		{
			name: "no diffs creates copy of base",
			setupBase: func(t *testing.T, baseDir string) {
				initGitRepo(t, baseDir)
				writeFile(t, filepath.Join(baseDir, "README.md"), "# Base\n")
				gitCommit(t, baseDir, "base commit")
			},
			diffs:   []string{},
			wantErr: false,
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "README.md"), "# Base\n")
				assertGitRepo(t, destDir)
			},
		},
		{
			name: "single diff applies correctly",
			setupBase: func(t *testing.T, baseDir string) {
				initGitRepo(t, baseDir)
				writeFile(t, filepath.Join(baseDir, "file1.txt"), "original\n")
				gitCommit(t, baseDir, "base commit")
			},
			diffs: []string{
				`diff --git a/file1.txt b/file1.txt
index 5802f8d..5be0cb3 100644
--- a/file1.txt
+++ b/file1.txt
@@ -1 +1 @@
-original
+modified
`,
			},
			wantErr: false,
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "file1.txt"), "modified\n")
			},
		},
		{
			name: "multiple ordered diffs apply sequentially",
			setupBase: func(t *testing.T, baseDir string) {
				initGitRepo(t, baseDir)
				writeFile(t, filepath.Join(baseDir, "counter.txt"), "0\n")
				gitCommit(t, baseDir, "base commit")
			},
			diffs: []string{
				// Diff 0: increment counter to 1.
				`diff --git a/counter.txt b/counter.txt
index 573541a..d00491f 100644
--- a/counter.txt
+++ b/counter.txt
@@ -1 +1 @@
-0
+1
`,
				// Diff 1: increment counter to 2.
				`diff --git a/counter.txt b/counter.txt
index d00491f..0cfbf08 100644
--- a/counter.txt
+++ b/counter.txt
@@ -1 +1 @@
-1
+2
`,
				// Diff 2: add new file.
				`diff --git a/added.txt b/added.txt
new file mode 100644
index 0000000..7898192
--- /dev/null
+++ b/added.txt
@@ -0,0 +1 @@
+hello
`,
			},
			wantErr: false,
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "counter.txt"), "2\n")
				assertFileContent(t, filepath.Join(destDir, "added.txt"), "hello\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create base clone directory.
			baseClone := t.TempDir()
			tt.setupBase(t, baseClone)

			// Create destination workspace directory.
			destWorkspace := t.TempDir()
			// Remove the empty dir created by TempDir so rehydration can create it.
			if err := os.Remove(destWorkspace); err != nil {
				t.Fatalf("failed to remove temp dir: %v", err)
			}

			// Gzip all diffs.
			gzippedDiffs := make([][]byte, len(tt.diffs))
			for i, diff := range tt.diffs {
				gzippedDiffs[i] = gzipBytes(t, []byte(diff))
			}

			// Execute rehydration.
			ctx := context.Background()
			err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, destWorkspace, gzippedDiffs)

			if (err != nil) != tt.wantErr {
				t.Errorf("RehydrateWorkspaceFromBaseAndDiffs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateAfter != nil {
				tt.validateAfter(t, destWorkspace)
			}
		})
	}
}

// TestCopyGitClone verifies git repository copying.
func TestCopyGitClone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupSrc     func(t *testing.T, srcDir string)
		wantErr      bool
		validateDest func(t *testing.T, destDir string)
	}{
		{
			name: "copy valid git repo",
			setupSrc: func(t *testing.T, srcDir string) {
				initGitRepo(t, srcDir)
				writeFile(t, filepath.Join(srcDir, "README.md"), "# Source\n")
				gitCommit(t, srcDir, "source commit")
			},
			wantErr: false,
			validateDest: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "README.md"), "# Source\n")
				assertGitRepo(t, destDir)
			},
		},
		{
			name: "copy fails for non-git directory",
			setupSrc: func(t *testing.T, srcDir string) {
				// Create a directory without .git.
				writeFile(t, filepath.Join(srcDir, "file.txt"), "content\n")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create source directory.
			src := t.TempDir()
			tt.setupSrc(t, src)

			// Create destination directory.
			dest := t.TempDir()
			// Remove the empty dir so copyGitClone can create it.
			if err := os.Remove(dest); err != nil {
				t.Fatalf("failed to remove temp dest dir: %v", err)
			}

			// Execute copy.
			err := copyGitClone(src, dest)

			if (err != nil) != tt.wantErr {
				t.Errorf("copyGitClone() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateDest != nil {
				tt.validateDest(t, dest)
			}
		})
	}
}

// --- Test Helpers ---

// gzipBytes compresses input bytes using gzip.
func gzipBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

// initGitRepo initializes a git repository in the specified directory.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (output: %s)", err, string(output))
	}
	// Configure git user for commits.
	configUser := exec.Command("git", "config", "user.name", "Test User")
	configUser.Dir = dir
	if output, err := configUser.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name failed: %v (output: %s)", err, string(output))
	}
	configEmail := exec.Command("git", "config", "user.email", "test@example.com")
	configEmail.Dir = dir
	if output, err := configEmail.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email failed: %v (output: %s)", err, string(output))
	}
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s failed: %v", path, err)
	}
}

// gitCommit stages all changes and creates a commit.
func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	add := exec.Command("git", "add", ".")
	add.Dir = dir
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v (output: %s)", err, string(output))
	}
	commit := exec.Command("git", "commit", "-m", message)
	commit.Dir = dir
	if output, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v (output: %s)", err, string(output))
	}
}

// assertFileContent verifies file content matches expected value.
func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s failed: %v", path, err)
	}
	if string(content) != expected {
		t.Errorf("file %s content = %q, want %q", path, string(content), expected)
	}
}

// assertGitRepo verifies the directory is a valid git repository.
func assertGitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("directory %s is not a git repo: %v", dir, err)
	}
}
