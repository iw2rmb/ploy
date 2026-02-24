package nodeagent

import (
	"bytes"
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

	// rsync is required for copyGitClone; skip if unavailable to avoid
	// environment-dependent failures on systems without rsync.
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
				// Simulate a rehydrated workspace with applied diffs.
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "base.txt"), "base content\n")
				gitCommit(t, dir, "base commit")
				// Apply changes (simulating rehydrated diffs).
				writeFile(t, filepath.Join(dir, "step0.txt"), "step 0 changes\n")
				writeFile(t, filepath.Join(dir, "step1.txt"), "step 1 changes\n")
			},
			wantErr: false,
			validateAfter: func(t *testing.T, dir string) {
				// Verify baseline commit was created.
				cmd := exec.Command("git", "log", "--oneline", "-1")
				cmd.Dir = dir
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git log failed: %v", err)
				}
				logOutput := string(output)
				if !bytes.Contains([]byte(logOutput), []byte("Ploy: rehydration baseline")) {
					t.Errorf("expected baseline commit message, got: %s", logOutput)
				}

				// Verify files were staged and committed.
				assertFileContent(t, filepath.Join(dir, "step0.txt"), "step 0 changes\n")
				assertFileContent(t, filepath.Join(dir, "step1.txt"), "step 1 changes\n")

				// Verify working tree is clean after commit.
				status := exec.Command("git", "status", "--porcelain")
				status.Dir = dir
				statusOut, err := status.CombinedOutput()
				if err != nil {
					t.Fatalf("git status failed: %v", err)
				}
				if len(statusOut) > 0 {
					t.Errorf("expected clean working tree, got: %s", string(statusOut))
				}
			},
		},
		{
			name: "handles workspace with no changes (no-op)",
			setupWorkspace: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "base.txt"), "base content\n")
				gitCommit(t, dir, "base commit")
			},
			wantErr:       false, // EnsureCommit returns (false, nil) when nothing to commit.
			validateAfter: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create workspace directory.
			workspace := t.TempDir()
			tt.setupWorkspace(t, workspace)

			// Execute baseline commit creation.
			ctx := context.Background()
			err := ensureBaselineCommitForRehydration(ctx, workspace)

			if (err != nil) != tt.wantErr {
				t.Errorf("ensureBaselineCommitForRehydration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

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

	// Scenario: Simulate a 3-step run where each step modifies the workspace.
	// For each step k:
	//   1. Rehydrate workspace from base + diffs[0..k-1].
	//   2. Create baseline commit (simulating the new behavior).
	//   3. Apply step k changes.
	//   4. Generate diff[k] using "git diff HEAD".
	//   5. Verify diff[k] contains only step k changes (not cumulative).
	//
	// Then verify that replaying diffs[0..2] on a fresh base clone reconstructs
	// the final workspace state.

	// Step 0: Create base clone.
	baseClone := t.TempDir()
	initGitRepo(t, baseClone)
	writeFile(t, filepath.Join(baseClone, "counter.txt"), "0\n")
	gitCommit(t, baseClone, "base commit")

	// Track diffs generated by each step.
	var generatedDiffs [][]byte

	// Step 0: Execute on base clone (no rehydration needed).
	{
		step0Workspace := t.TempDir()
		if err := os.Remove(step0Workspace); err != nil {
			t.Fatalf("failed to remove temp dir: %v", err)
		}
		if err := copyGitClone(baseClone, step0Workspace); err != nil {
			t.Fatalf("copy base clone for step 0: %v", err)
		}

		// Simulate step 0 execution: increment counter to 1.
		writeFile(t, filepath.Join(step0Workspace, "counter.txt"), "1\n")

		// Generate diff for step 0 using "git diff HEAD".
		diff0 := generateGitDiff(t, step0Workspace)
		generatedDiffs = append(generatedDiffs, diff0)

		// Verify diff 0 contains incremental changes (0 -> 1).
		if !bytes.Contains(diff0, []byte("-0")) || !bytes.Contains(diff0, []byte("+1")) {
			t.Errorf("step 0 diff should contain 0->1 change, got:\n%s", string(diff0))
		}
	}

	// Step 1: Rehydrate from base + diff[0], create baseline commit, execute.
	{
		step1Workspace := t.TempDir()
		if err := os.Remove(step1Workspace); err != nil {
			t.Fatalf("failed to remove temp dir: %v", err)
		}

		// Rehydrate: base + diff[0].
		gzippedDiffs := [][]byte{gzipBytes(t, generatedDiffs[0])}
		if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, step1Workspace, gzippedDiffs); err != nil {
			t.Fatalf("rehydrate for step 1: %v", err)
		}

		// Create baseline commit (NEW BEHAVIOR).
		if err := ensureBaselineCommitForRehydration(ctx, step1Workspace); err != nil {
			t.Fatalf("baseline commit for step 1: %v", err)
		}

		// Simulate step 1 execution: increment counter to 2.
		writeFile(t, filepath.Join(step1Workspace, "counter.txt"), "2\n")

		// Generate diff for step 1 using "git diff HEAD".
		diff1 := generateGitDiff(t, step1Workspace)
		generatedDiffs = append(generatedDiffs, diff1)

		// Verify diff 1 contains ONLY incremental changes (1 -> 2), NOT cumulative (0 -> 2).
		if bytes.Contains(diff1, []byte("-0")) {
			t.Errorf("step 1 diff should NOT contain base state (0), got:\n%s", string(diff1))
		}
		if !bytes.Contains(diff1, []byte("-1")) || !bytes.Contains(diff1, []byte("+2")) {
			t.Errorf("step 1 diff should contain 1->2 change, got:\n%s", string(diff1))
		}
	}

	// Step 2: Rehydrate from base + diff[0..1], create baseline commit, execute.
	{
		step2Workspace := t.TempDir()
		if err := os.Remove(step2Workspace); err != nil {
			t.Fatalf("failed to remove temp dir: %v", err)
		}

		// Rehydrate: base + diff[0] + diff[1].
		gzippedDiffs := [][]byte{
			gzipBytes(t, generatedDiffs[0]),
			gzipBytes(t, generatedDiffs[1]),
		}
		if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, step2Workspace, gzippedDiffs); err != nil {
			t.Fatalf("rehydrate for step 2: %v", err)
		}

		// Create baseline commit (NEW BEHAVIOR).
		if err := ensureBaselineCommitForRehydration(ctx, step2Workspace); err != nil {
			t.Fatalf("baseline commit for step 2: %v", err)
		}

		// Simulate step 2 execution: increment counter to 3 and add new file.
		writeFile(t, filepath.Join(step2Workspace, "counter.txt"), "3\n")
		writeFile(t, filepath.Join(step2Workspace, "added.txt"), "hello from step 2\n")

		// Stage the new file so it appears in the diff.
		// In actual execution, files are created by the container and will be part of the working tree.
		// git diff HEAD captures both tracked changes and staged new files.
		addCmd := exec.Command("git", "add", "added.txt")
		addCmd.Dir = step2Workspace
		if output, err := addCmd.CombinedOutput(); err != nil {
			t.Fatalf("git add for step 2: %v (output: %s)", err, string(output))
		}

		// Generate diff for step 2 using "git diff HEAD".
		diff2 := generateGitDiff(t, step2Workspace)
		generatedDiffs = append(generatedDiffs, diff2)

		// Verify diff 2 contains ONLY incremental changes (2 -> 3 + added.txt).
		// Check that the diff doesn't reference old states (0 or 1) in counter.txt changes.
		diff2Str := string(diff2)

		// The diff should contain counter.txt changes (2 -> 3).
		if !bytes.Contains(diff2, []byte("-2")) || !bytes.Contains(diff2, []byte("+3")) {
			t.Errorf("step 2 diff should contain 2->3 change, got:\n%s", diff2Str)
		}

		// The diff should NOT contain cumulative changes from base (0 or 1).
		// Note: "0" and "1" might appear in file mode/index lines, so check in context.
		if bytes.Contains(diff2, []byte("-0\n")) || bytes.Contains(diff2, []byte("-1\n")) {
			t.Errorf("step 2 diff should NOT contain prior counter state (0 or 1), got:\n%s", diff2Str)
		}

		// The diff should contain new file added.txt.
		if !bytes.Contains(diff2, []byte("+hello from step 2")) {
			t.Errorf("step 2 diff should contain new file added.txt, got:\n%s", diff2Str)
		}
	}

	// Final verification: Rehydrate a fresh workspace using all diffs[0..2] and verify final state.
	{
		finalWorkspace := t.TempDir()
		if err := os.Remove(finalWorkspace); err != nil {
			t.Fatalf("failed to remove temp dir: %v", err)
		}

		// Rehydrate: base + diff[0] + diff[1] + diff[2].
		gzippedDiffs := [][]byte{
			gzipBytes(t, generatedDiffs[0]),
			gzipBytes(t, generatedDiffs[1]),
			gzipBytes(t, generatedDiffs[2]),
		}
		if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, finalWorkspace, gzippedDiffs); err != nil {
			t.Fatalf("rehydrate final workspace: %v", err)
		}

		// Verify final state matches expected: counter=3, added.txt exists.
		assertFileContent(t, filepath.Join(finalWorkspace, "counter.txt"), "3\n")
		assertFileContent(t, filepath.Join(finalWorkspace, "added.txt"), "hello from step 2\n")
	}
}
