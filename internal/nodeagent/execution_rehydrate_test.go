package nodeagent

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
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
		},
		{
			name:       "valid gzipped patch",
			input:      gzipBytes(t, []byte("diff --git a/file.txt b/file.txt")),
			wantOutput: "diff --git a/file.txt b/file.txt",
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
			checkErr(t, tt.wantErr, err)
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
		patch         string
		wantErr       bool
		validateAfter func(t *testing.T, dir string)
	}{
		{
			name:  "empty patch applies cleanly",
			patch: "",
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "README.md"), "# Test\n")
			},
		},
		{
			name: "valid patch adds new file",
			patch: `diff --git a/newfile.txt b/newfile.txt
new file mode 100644
index 0000000..ce01362
--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1 @@
+hello
`,
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "newfile.txt"), "hello\n")
			},
		},
		{
			name: "valid patch modifies existing file",
			patch: `diff --git a/README.md b/README.md
index 2a02d41..0527e6b 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-# Test
+# Test Modified
`,
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "README.md"), "# Test Modified\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workspace := t.TempDir()
			initRepoWithFile(t, workspace, "README.md", "# Test\n")

			ctx := context.Background()
			err := applyGzippedPatch(ctx, workspace, gzipBytes(t, []byte(tt.patch)))
			checkErr(t, tt.wantErr, err)

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
		baseFile      string
		baseContent   string
		diffs         []string
		wantErr       bool
		validateAfter func(t *testing.T, destDir string)
	}{
		{
			name:        "no diffs creates copy of base",
			baseFile:    "README.md",
			baseContent: "# Base\n",
			diffs:       []string{},
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "README.md"), "# Base\n")
				assertGitRepo(t, destDir)
			},
		},
		{
			name:        "single diff applies correctly",
			baseFile:    "file1.txt",
			baseContent: "original\n",
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
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "file1.txt"), "modified\n")
			},
		},
		{
			name:        "multiple ordered diffs apply sequentially",
			baseFile:    "counter.txt",
			baseContent: "0\n",
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
			validateAfter: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "counter.txt"), "2\n")
				assertFileContent(t, filepath.Join(destDir, "added.txt"), "hello\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseClone := t.TempDir()
			initRepoWithFile(t, baseClone, tt.baseFile, tt.baseContent)

			destWorkspace := removedTempDir(t)

			gzippedDiffs := make([][]byte, len(tt.diffs))
			for i, diff := range tt.diffs {
				gzippedDiffs[i] = gzipBytes(t, []byte(diff))
			}

			ctx := context.Background()
			err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, destWorkspace, gzippedDiffs)
			checkErr(t, tt.wantErr, err)

			if !tt.wantErr && tt.validateAfter != nil {
				tt.validateAfter(t, destWorkspace)
			}
		})
	}
}

// TestCopyGitClone verifies git repository copying.
func TestCopyGitClone(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not found, skipping TestCopyGitClone")
	}

	tests := []struct {
		name         string
		setupSrc     func(t *testing.T, srcDir string)
		wantErr      bool
		validateDest func(t *testing.T, destDir string)
	}{
		{
			name: "copy valid git repo",
			setupSrc: func(t *testing.T, srcDir string) {
				initRepoWithFile(t, srcDir, "README.md", "# Source\n")
			},
			validateDest: func(t *testing.T, destDir string) {
				assertFileContent(t, filepath.Join(destDir, "README.md"), "# Source\n")
				assertGitRepo(t, destDir)
			},
		},
		{
			name: "copy fails for non-git directory",
			setupSrc: func(t *testing.T, srcDir string) {
				writeFile(t, filepath.Join(srcDir, "file.txt"), "content\n")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := t.TempDir()
			tt.setupSrc(t, src)

			dest := removedTempDir(t)
			err := copyGitClone(src, dest)
			checkErr(t, tt.wantErr, err)

			if !tt.wantErr && tt.validateDest != nil {
				tt.validateDest(t, dest)
			}
		})
	}
}

// TestEnsureBaselineCommitForRehydration verifies baseline commit creation after rehydration.
func TestEnsureBaselineCommitForRehydration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupWorkspace func(t *testing.T, dir string)
		wantErr        bool
		validateAfter  func(t *testing.T, dir string)
	}{
		{
			name: "creates commit with rehydrated changes",
			setupWorkspace: func(t *testing.T, dir string) {
				initRepoWithFile(t, dir, "base.txt", "base content\n")
				writeFile(t, filepath.Join(dir, "step0.txt"), "step 0 changes\n")
				writeFile(t, filepath.Join(dir, "step1.txt"), "step 1 changes\n")
			},
			validateAfter: func(t *testing.T, dir string) {
				logOut := string(gitrepo.Run(t, dir, "log", "--oneline", "-1"))
				if !strings.Contains(logOut, "Ploy: rehydration baseline") {
					t.Errorf("expected baseline commit message, got: %s", logOut)
				}
				assertFileContent(t, filepath.Join(dir, "step0.txt"), "step 0 changes\n")
				assertFileContent(t, filepath.Join(dir, "step1.txt"), "step 1 changes\n")
				statusOut := string(gitrepo.Run(t, dir, "status", "--porcelain"))
				if len(statusOut) > 0 {
					t.Errorf("expected clean working tree, got: %s", statusOut)
				}
			},
		},
		{
			name: "handles workspace with no changes (no-op)",
			setupWorkspace: func(t *testing.T, dir string) {
				initRepoWithFile(t, dir, "base.txt", "base content\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workspace := t.TempDir()
			tt.setupWorkspace(t, workspace)

			ctx := context.Background()
			err := ensureBaselineCommitForRehydration(ctx, workspace)
			checkErr(t, tt.wantErr, err)

			if !tt.wantErr && tt.validateAfter != nil {
				tt.validateAfter(t, workspace)
			}
		})
	}
}

// TestIncrementalDiffsAreRehydrationSafe verifies that per-step diffs are incremental
// and can be replayed in order to reconstruct workspace state.
func TestIncrementalDiffsAreRehydrationSafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create base clone with counter=0.
	baseClone := t.TempDir()
	initRepoWithFile(t, baseClone, "counter.txt", "0\n")

	type incrementalStep struct {
		counterValue   string
		extraFiles     map[string]string
		gitAddFiles    []string
		mustContain    []string
		mustNotContain []string
	}

	steps := []incrementalStep{
		{
			counterValue: "1\n",
			mustContain:  []string{"-0", "+1"},
		},
		{
			counterValue:   "2\n",
			mustContain:    []string{"-1", "+2"},
			mustNotContain: []string{"-0"},
		},
		{
			counterValue:   "3\n",
			extraFiles:     map[string]string{"added.txt": "hello from step 2\n"},
			gitAddFiles:    []string{"added.txt"},
			mustContain:    []string{"-2", "+3", "+hello from step 2"},
			mustNotContain: []string{"-0\n", "-1\n"},
		},
	}

	var generatedDiffs [][]byte

	for i, step := range steps {
		workspace := removedTempDir(t)

		if i == 0 {
			if err := copyGitClone(baseClone, workspace); err != nil {
				t.Fatalf("step %d: copy base: %v", i, err)
			}
		} else {
			gzipped := make([][]byte, len(generatedDiffs))
			for j, d := range generatedDiffs {
				gzipped[j] = gzipBytes(t, d)
			}
			if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, workspace, gzipped); err != nil {
				t.Fatalf("step %d: rehydrate: %v", i, err)
			}
			if err := ensureBaselineCommitForRehydration(ctx, workspace); err != nil {
				t.Fatalf("step %d: baseline commit: %v", i, err)
			}
		}

		// Apply step changes.
		writeFile(t, filepath.Join(workspace, "counter.txt"), step.counterValue)
		for file, content := range step.extraFiles {
			writeFile(t, filepath.Join(workspace, file), content)
		}
		for _, file := range step.gitAddFiles {
			gitRun(t, workspace, "add", file)
		}

		// Generate and collect diff.
		diff := generateGitDiff(t, workspace)
		generatedDiffs = append(generatedDiffs, diff)

		// Assert diff contents.
		for _, s := range step.mustContain {
			if !bytes.Contains(diff, []byte(s)) {
				t.Errorf("step %d: diff should contain %q, got:\n%s", i, s, diff)
			}
		}
		for _, s := range step.mustNotContain {
			if bytes.Contains(diff, []byte(s)) {
				t.Errorf("step %d: diff should NOT contain %q, got:\n%s", i, s, diff)
			}
		}
	}

	// Final: replay all diffs on fresh base and verify state.
	finalWorkspace := removedTempDir(t)
	gzipped := make([][]byte, len(generatedDiffs))
	for i, d := range generatedDiffs {
		gzipped[i] = gzipBytes(t, d)
	}
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, finalWorkspace, gzipped); err != nil {
		t.Fatalf("final rehydrate: %v", err)
	}
	assertFileContent(t, filepath.Join(finalWorkspace, "counter.txt"), "3\n")
	assertFileContent(t, filepath.Join(finalWorkspace, "added.txt"), "hello from step 2\n")
}
