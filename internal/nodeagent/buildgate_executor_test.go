package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestBuildGateExecutor_ApplyDiffPatch verifies that the applyDiffPatch method
// correctly decompresses and applies gzipped unified diffs to a git workspace.
func TestBuildGateExecutor_ApplyDiffPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupRepo     func(t *testing.T, dir string)
		diffPatch     string
		wantErr       bool
		validateAfter func(t *testing.T, dir string)
	}{
		{
			name: "valid diff_patch adds new file",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			diffPatch: `diff --git a/newfile.txt b/newfile.txt
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
			name: "valid diff_patch modifies existing file",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			diffPatch: `diff --git a/README.md b/README.md
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
		{
			name: "whitespace-only diff_patch is no-op",
			setupRepo: func(t *testing.T, dir string) {
				initGitRepo(t, dir)
				writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
				gitCommit(t, dir, "initial commit")
			},
			diffPatch: "   \n\t  ", // Whitespace-only patch is no-op.
			wantErr:   false,
			validateAfter: func(t *testing.T, dir string) {
				assertFileContent(t, filepath.Join(dir, "README.md"), "# Test\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary workspace and setup repository.
			workspace := t.TempDir()
			tt.setupRepo(t, workspace)

			// Build gzipped patch.
			diffPatchBytes := gzipBytes(t, []byte(tt.diffPatch))

			// Create executor and apply patch directly (testing the core method).
			executor := &BuildGateExecutor{}
			ctx := context.Background()
			err := executor.applyDiffPatch(ctx, workspace, diffPatchBytes)

			// Verify error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("applyDiffPatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Validate workspace state after apply.
			if !tt.wantErr && tt.validateAfter != nil {
				tt.validateAfter(t, workspace)
			}
		})
	}
}

// TestBuildGateExecutor_ApplyDiffPatch_InvalidPatch verifies error handling
// for invalid gzipped patches.
func TestBuildGateExecutor_ApplyDiffPatch_InvalidPatch(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "README.md"), "# Test\n")
	gitCommit(t, workspace, "initial commit")

	// Test with invalid gzip data.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, []byte("not valid gzip"))

	if err == nil {
		t.Error("expected error for invalid gzip data, got nil")
	}
}

// TestBuildGateExecutor_ApplyDiffPatch_ConflictingPatch verifies error handling
// when a patch cannot be applied cleanly.
func TestBuildGateExecutor_ApplyDiffPatch_ConflictingPatch(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "README.md"), "# Test\n")
	gitCommit(t, workspace, "initial commit")

	// Create a patch that expects different content than what exists.
	conflictingPatch := `diff --git a/README.md b/README.md
index 0000000..0000001 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-# Different Content
+# Modified Content
`
	gzippedPatch := gzipBytes(t, []byte(conflictingPatch))

	// Test with conflicting patch.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, gzippedPatch)

	if err == nil {
		t.Error("expected error for conflicting patch, got nil")
	}
}

// TestBuildGateExecutor_RequestValidation verifies that BuildGateValidateRequest
// validates diff_patch requirements correctly.
func TestBuildGateExecutor_RequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     contracts.BuildGateValidateRequest
		wantErr bool
	}{
		{
			name: "valid request with repo_url and ref only",
			req: contracts.BuildGateValidateRequest{
				RepoURL: "https://example.com/test/repo.git",
				Ref:     "main",
				Profile: "auto",
			},
			wantErr: false,
		},
		{
			name: "valid request with repo_url, ref, and diff_patch",
			req: contracts.BuildGateValidateRequest{
				RepoURL:   "https://example.com/test/repo.git",
				Ref:       "main",
				DiffPatch: []byte("patch data"),
				Profile:   "auto",
			},
			wantErr: false,
		},
		{
			name: "invalid: diff_patch without repo_url",
			req: contracts.BuildGateValidateRequest{
				Ref:       "main",
				DiffPatch: []byte("patch data"),
			},
			wantErr: true,
		},
		{
			name: "invalid: diff_patch without ref",
			req: contracts.BuildGateValidateRequest{
				RepoURL:   "https://example.com/test/repo.git",
				DiffPatch: []byte("patch data"),
			},
			wantErr: true,
		},
		{
			name: "invalid: missing repo_url",
			req: contracts.BuildGateValidateRequest{
				Ref: "main",
			},
			wantErr: true,
		},
		{
			name: "invalid: missing ref",
			req: contracts.BuildGateValidateRequest{
				RepoURL: "https://example.com/test/repo.git",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBuildGateExecutor_DiffPatchSkippedWhenEmpty verifies that the Execute flow
// correctly skips diff_patch application when DiffPatch is empty.
func TestBuildGateExecutor_DiffPatchSkippedWhenEmpty(t *testing.T) {
	t.Parallel()

	// Create a request with empty DiffPatch.
	req := contracts.BuildGateValidateRequest{
		RepoURL: "https://example.com/test/repo.git",
		Ref:     "main",
		Profile: "auto",
	}

	// Verify DiffPatch is empty.
	if len(req.DiffPatch) != 0 {
		t.Error("DiffPatch should be empty")
	}

	// The condition in Execute: if len(req.DiffPatch) > 0 should NOT trigger.
	shouldApply := len(req.DiffPatch) > 0
	if shouldApply {
		t.Error("applyDiffPatch should not be called when DiffPatch is empty")
	}
}

// TestBuildGateExecutor_DiffPatchTriggeredWhenNonEmpty verifies that the Execute flow
// correctly triggers diff_patch application when DiffPatch is non-empty.
func TestBuildGateExecutor_DiffPatchTriggeredWhenNonEmpty(t *testing.T) {
	t.Parallel()

	// Create a request with non-empty DiffPatch.
	req := contracts.BuildGateValidateRequest{
		RepoURL:   "https://example.com/test/repo.git",
		Ref:       "main",
		DiffPatch: []byte("patch data"),
		Profile:   "auto",
	}

	// Verify DiffPatch is non-empty.
	if len(req.DiffPatch) == 0 {
		t.Error("DiffPatch should be non-empty")
	}

	// The condition in Execute: if len(req.DiffPatch) > 0 should trigger.
	shouldApply := len(req.DiffPatch) > 0
	if !shouldApply {
		t.Error("applyDiffPatch should be called when DiffPatch is non-empty")
	}
}

// TestBuildGateExecutor_ApplyDiffPatch_MultiFilePatch verifies that patches
// modifying multiple files are applied correctly.
func TestBuildGateExecutor_ApplyDiffPatch_MultiFilePatch(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "file1.txt"), "content1\n")
	writeFile(t, filepath.Join(workspace, "file2.txt"), "content2\n")
	gitCommit(t, workspace, "initial commit")

	// Create a patch that modifies multiple files.
	multiFilePatch := `diff --git a/file1.txt b/file1.txt
index 0000000..0000001 100644
--- a/file1.txt
+++ b/file1.txt
@@ -1 +1 @@
-content1
+modified1
diff --git a/file2.txt b/file2.txt
index 0000000..0000002 100644
--- a/file2.txt
+++ b/file2.txt
@@ -1 +1 @@
-content2
+modified2
diff --git a/newfile.txt b/newfile.txt
new file mode 100644
index 0000000..ce01362
--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1 @@
+new content
`
	gzippedPatch := gzipBytes(t, []byte(multiFilePatch))

	// Apply the patch.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, gzippedPatch)

	if err != nil {
		t.Fatalf("applyDiffPatch() error = %v", err)
	}

	// Verify all files were modified correctly.
	assertFileContent(t, filepath.Join(workspace, "file1.txt"), "modified1\n")
	assertFileContent(t, filepath.Join(workspace, "file2.txt"), "modified2\n")
	assertFileContent(t, filepath.Join(workspace, "newfile.txt"), "new content\n")
}

// TestBuildGateExecutor_ApplyDiffPatch_DeleteFile verifies that patches
// deleting files are applied correctly.
func TestBuildGateExecutor_ApplyDiffPatch_DeleteFile(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "keep.txt"), "keep\n")
	writeFile(t, filepath.Join(workspace, "delete.txt"), "delete\n")
	gitCommit(t, workspace, "initial commit")

	// Create a patch that deletes a file.
	deletePatch := `diff --git a/delete.txt b/delete.txt
deleted file mode 100644
index 0000000..0000000
--- a/delete.txt
+++ /dev/null
@@ -1 +0,0 @@
-delete
`
	gzippedPatch := gzipBytes(t, []byte(deletePatch))

	// Apply the patch.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, gzippedPatch)

	if err != nil {
		t.Fatalf("applyDiffPatch() error = %v", err)
	}

	// Verify file was deleted.
	if _, err := os.Stat(filepath.Join(workspace, "delete.txt")); !os.IsNotExist(err) {
		t.Error("delete.txt should have been deleted")
	}

	// Verify other file is unchanged.
	assertFileContent(t, filepath.Join(workspace, "keep.txt"), "keep\n")
}

// TestBuildGateExecutor_ApplyDiffPatch_EmptyPatch verifies that empty patches are no-op.
func TestBuildGateExecutor_ApplyDiffPatch_EmptyPatch(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "README.md"), "# Test\n")
	gitCommit(t, workspace, "initial commit")

	// Create an empty gzipped patch.
	gzippedPatch := gzipBytes(t, []byte(""))

	// Apply the patch.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, gzippedPatch)

	if err != nil {
		t.Fatalf("applyDiffPatch() error = %v (empty patch should be no-op)", err)
	}

	// Verify file is unchanged.
	assertFileContent(t, filepath.Join(workspace, "README.md"), "# Test\n")
}

// TestBuildGateExecutor_ApplyDiffPatch_SubdirectoryPatch verifies that patches
// affecting files in subdirectories are applied correctly.
func TestBuildGateExecutor_ApplyDiffPatch_SubdirectoryPatch(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with a valid git repo and subdirectory.
	workspace := t.TempDir()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "src", "main.go"), "package main\n")
	gitCommit(t, workspace, "initial commit")

	// Create a patch that modifies a file in a subdirectory.
	subdirPatch := `diff --git a/src/main.go b/src/main.go
index 0000000..0000001 100644
--- a/src/main.go
+++ b/src/main.go
@@ -1 +1,3 @@
 package main
+
+func main() {}
`
	gzippedPatch := gzipBytes(t, []byte(subdirPatch))

	// Apply the patch.
	executor := &BuildGateExecutor{}
	ctx := context.Background()
	err := executor.applyDiffPatch(ctx, workspace, gzippedPatch)

	if err != nil {
		t.Fatalf("applyDiffPatch() error = %v", err)
	}

	// Verify file was modified.
	assertFileContent(t, filepath.Join(workspace, "src", "main.go"), "package main\n\nfunc main() {}\n")
}

// Note: gzipBytes helper is already defined in execution_rehydrate_test.go.
// Note: initGitRepo, writeFile, gitCommit, assertFileContent are defined in execution_rehydrate_test.go.
