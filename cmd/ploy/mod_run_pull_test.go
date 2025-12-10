package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestModRunPullRouting validates that `ploy mod run pull` routes to handleModRunPull.
// These tests run in a clean git repository to satisfy the git worktree preconditions.
func TestModRunPullRouting(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	tests := []struct {
		name       string
		args       []string
		wantErr    string
		wantOutput string
		needsRepo  bool   // true if test needs a clean git repo with origin
		needRemote string // remote name needed (empty = use "origin")
	}{
		{
			name:    "pull without run-name",
			args:    []string{"mod", "run", "pull"},
			wantErr: "run-name or run-id required",
		},
		{
			name:    "pull with empty run-name",
			args:    []string{"mod", "run", "pull", "   "},
			wantErr: "run-name or run-id required",
		},
		// Note: Tests that previously checked for placeholder output are now
		// integration tests that require a running control plane. Since the
		// implementation makes API calls to resolve runs, these tests are
		// expected to fail with API errors when no control plane is configured.
		// The detailed API interaction tests are in internal/cli/mods/repos_test.go.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup: if the test needs a repo, create one and chdir into it.
			var cleanup func()
			if tc.needsRepo {
				repoDir := setupTestGitRepoForPull(t)
				// Add appropriate remote.
				remoteName := "origin"
				if tc.needRemote != "" {
					remoteName = tc.needRemote
				}
				runGitCmdForPull(t, repoDir, "remote", "add", remoteName, "https://github.com/example/repo.git")

				origDir, err := os.Getwd()
				if err != nil {
					t.Fatalf("failed to get current directory: %v", err)
				}
				cleanup = func() { _ = os.Chdir(origDir) }
				if err := os.Chdir(repoDir); err != nil {
					t.Fatalf("failed to change to repo directory: %v", err)
				}
			}
			defer func() {
				if cleanup != nil {
					cleanup()
				}
			}()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			if tc.wantOutput != "" && !strings.Contains(output, tc.wantOutput) {
				t.Errorf("output %q should contain %q", output, tc.wantOutput)
			}
		})
	}
}

// TestModRunPullUsageErrors validates that invalid flag combinations return appropriate errors.
// Uses t.Parallel since it does not use t.Setenv.
func TestModRunPullUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantUsage bool // expect usage text in output
	}{
		{
			name:      "unknown flag",
			args:      []string{"mod", "run", "pull", "--unknown", "my-run"},
			wantErr:   "flag provided but not defined",
			wantUsage: true,
		},
		{
			name:      "origin flag without value",
			args:      []string{"mod", "run", "pull", "--origin"},
			wantErr:   "flag needs an argument",
			wantUsage: true,
		},
		{
			name:    "extra positional argument",
			args:    []string{"mod", "run", "pull", "my-run", "extra-arg"},
			wantErr: "unexpected argument: extra-arg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}

			if tc.wantUsage {
				output := buf.String()
				if !strings.Contains(output, "Usage: ploy mod run pull") {
					t.Errorf("expected usage output, got %q", output)
				}
			}
		})
	}
}

// TestModRunPullUsageHelp validates that the usage text contains expected content.
func TestModRunPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printModRunPullUsage(&buf)

	output := buf.String()

	// Verify usage line is present.
	if !strings.Contains(output, "Usage: ploy mod run pull") {
		t.Errorf("usage should contain command line, got %q", output)
	}

	// Verify flags are documented.
	if !strings.Contains(output, "--origin") {
		t.Errorf("usage should document --origin flag, got %q", output)
	}
	if !strings.Contains(output, "--dry-run") {
		t.Errorf("usage should document --dry-run flag, got %q", output)
	}

	// Verify argument is documented.
	if !strings.Contains(output, "<run-name|run-id>") {
		t.Errorf("usage should document run-name|run-id argument, got %q", output)
	}

	// Verify examples are present.
	if !strings.Contains(output, "Examples:") {
		t.Errorf("usage should contain examples section, got %q", output)
	}
}

// TestModRunPullDefaultOrigin validates that origin defaults to "origin" and
// that git worktree validation passes before API calls are made.
// This test runs in a controlled git repository environment.
// Note: This test expects an API error since no control plane is available,
// but that error should reference the origin URL, confirming the default is "origin".
func TestModRunPullDefaultOrigin(t *testing.T) {
	// Skip: This test requires a running control plane to fully validate.
	// The default origin behavior is validated implicitly by the API call URL.
	// For unit testing the default origin, see the resolveGitRemoteURL tests.
	t.Skip("requires running control plane; default origin validated via API URL")
}

// =============================================================================
// Git Worktree Detection Tests
// =============================================================================

// TestEnsureInsideGitWorktree_InsideRepo verifies that ensureInsideGitWorktree
// succeeds when called from inside a git repository.
func TestEnsureInsideGitWorktree_InsideRepo(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a temporary git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Change to the repo directory and run the test.
	// Note: We save and restore the original working directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ensureInsideGitWorktree(ctx); err != nil {
		t.Errorf("ensureInsideGitWorktree() inside repo should succeed, got error: %v", err)
	}
}

// TestEnsureInsideGitWorktree_OutsideRepo verifies that ensureInsideGitWorktree
// returns an error when called from outside a git repository.
func TestEnsureInsideGitWorktree_OutsideRepo(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a temporary directory that is NOT a git repository.
	nonRepoDir := t.TempDir()

	// Change to the non-repo directory and run the test.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(nonRepoDir); err != nil {
		t.Fatalf("failed to change to non-repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = ensureInsideGitWorktree(ctx)
	if err == nil {
		t.Error("ensureInsideGitWorktree() outside repo should return error")
	}
	if !strings.Contains(err.Error(), "must be run inside a git repository") {
		t.Errorf("error should mention 'must be run inside a git repository', got: %v", err)
	}
}

// =============================================================================
// Clean Working Tree Tests
// =============================================================================

// TestEnsureCleanWorkingTree_Clean verifies that ensureCleanWorkingTree
// succeeds when the working tree has no uncommitted changes.
func TestEnsureCleanWorkingTree_Clean(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a clean temporary git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ensureCleanWorkingTree(ctx); err != nil {
		t.Errorf("ensureCleanWorkingTree() on clean repo should succeed, got error: %v", err)
	}
}

// TestEnsureCleanWorkingTree_UntrackedFiles verifies that ensureCleanWorkingTree
// returns an error when there are untracked files in the working tree.
func TestEnsureCleanWorkingTree_UntrackedFiles(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with an untracked file.
	repoDir := setupTestGitRepoForPull(t)

	// Add an untracked file.
	untrackedFile := filepath.Join(repoDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked content\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = ensureCleanWorkingTree(ctx)
	if err == nil {
		t.Error("ensureCleanWorkingTree() with untracked files should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// TestEnsureCleanWorkingTree_ModifiedFiles verifies that ensureCleanWorkingTree
// returns an error when there are modified files in the working tree.
func TestEnsureCleanWorkingTree_ModifiedFiles(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Modify an existing tracked file.
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Modified content\n"), 0644); err != nil {
		t.Fatalf("failed to modify README: %v", err)
	}

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = ensureCleanWorkingTree(ctx)
	if err == nil {
		t.Error("ensureCleanWorkingTree() with modified files should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// TestEnsureCleanWorkingTree_StagedFiles verifies that ensureCleanWorkingTree
// returns an error when there are staged but uncommitted changes.
func TestEnsureCleanWorkingTree_StagedFiles(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Create and stage a new file.
	newFile := filepath.Join(repoDir, "staged.txt")
	if err := os.WriteFile(newFile, []byte("staged content\n"), 0644); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}
	runGitCmdForPull(t, repoDir, "add", "staged.txt")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = ensureCleanWorkingTree(ctx)
	if err == nil {
		t.Error("ensureCleanWorkingTree() with staged files should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// =============================================================================
// Git Remote Resolution Tests
// =============================================================================

// TestResolveGitRemoteURL_ExistingRemote verifies that resolveGitRemoteURL
// successfully retrieves the URL for an existing remote.
func TestResolveGitRemoteURL_ExistingRemote(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with a remote.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := resolveGitRemoteURL(ctx, "origin")
	if err != nil {
		t.Fatalf("resolveGitRemoteURL() should succeed for existing remote, got error: %v", err)
	}
	if url != "https://github.com/example/repo.git" {
		t.Errorf("expected URL 'https://github.com/example/repo.git', got %q", url)
	}
}

// TestResolveGitRemoteURL_MissingRemote verifies that resolveGitRemoteURL
// returns an error when the specified remote does not exist.
func TestResolveGitRemoteURL_MissingRemote(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository without the "upstream" remote.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = resolveGitRemoteURL(ctx, "upstream")
	if err == nil {
		t.Error("resolveGitRemoteURL() should return error for missing remote")
	}
	if !strings.Contains(err.Error(), `git remote "upstream" not found`) {
		t.Errorf("error should mention 'git remote \"upstream\" not found', got: %v", err)
	}
}

// TestResolveGitRemoteURL_CustomOrigin verifies that resolveGitRemoteURL
// correctly resolves custom remote names (not just "origin").
func TestResolveGitRemoteURL_CustomOrigin(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with a custom remote.
	repoDir := setupTestGitRepoForPull(t)
	runGitCmdForPull(t, repoDir, "remote", "add", "upstream", "git@github.com:upstream/repo.git")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := resolveGitRemoteURL(ctx, "upstream")
	if err != nil {
		t.Fatalf("resolveGitRemoteURL() should succeed for custom remote, got error: %v", err)
	}
	if url != "git@github.com:upstream/repo.git" {
		t.Errorf("expected URL 'git@github.com:upstream/repo.git', got %q", url)
	}
}

// =============================================================================
// URL Normalization Tests
// =============================================================================

// TestNormalizeRepoURLForCLI validates the URL normalization function.
func TestNormalizeRepoURLForCLI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes trailing .git suffix",
			input:    "https://github.com/org/repo.git",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "removes trailing slash",
			input:    "https://github.com/org/repo/",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "removes both trailing slash and .git",
			input:    "https://github.com/org/repo.git/",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "trims whitespace",
			input:    "  https://github.com/org/repo  ",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "handles SSH URLs with .git suffix",
			input:    "git@github.com:org/repo.git",
			expected: "git@github.com:org/repo",
		},
		{
			name:     "leaves clean URL unchanged",
			input:    "https://github.com/org/repo",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "handles file:// URLs",
			input:    "file:///path/to/repo.git",
			expected: "file:///path/to/repo",
		},
		{
			name:     "preserves scheme and host",
			input:    "https://gitlab.example.com/org/repo.git",
			expected: "https://gitlab.example.com/org/repo",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "handles whitespace-only string",
			input:    "   ",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := normalizeRepoURLForCLI(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeRepoURLForCLI(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestNormalizeRepoURLForCLI_Consistency verifies that the normalization produces
// consistent results for URLs that should be considered equivalent.
func TestNormalizeRepoURLForCLI_Consistency(t *testing.T) {
	t.Parallel()

	// These URLs should all normalize to the same value.
	equivalentURLs := []string{
		"https://github.com/org/repo",
		"https://github.com/org/repo/",
		"https://github.com/org/repo.git",
		"https://github.com/org/repo.git/",
		"  https://github.com/org/repo  ",
		"  https://github.com/org/repo.git  ",
	}

	expected := normalizeRepoURLForCLI(equivalentURLs[0])
	for _, url := range equivalentURLs[1:] {
		result := normalizeRepoURLForCLI(url)
		if result != expected {
			t.Errorf("normalizeRepoURLForCLI(%q) = %q, expected %q (should match first URL)", url, result, expected)
		}
	}
}

// =============================================================================
// Integration Tests - handleModRunPull with Git Validation
// =============================================================================

// TestHandleModRunPull_OutsideGitRepo verifies that handleModRunPull fails
// when called outside a git repository.
func TestHandleModRunPull_OutsideGitRepo(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a temporary directory that is NOT a git repository.
	nonRepoDir := t.TempDir()

	// Change to the non-repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(nonRepoDir); err != nil {
		t.Fatalf("failed to change to non-repo directory: %v", err)
	}

	var buf bytes.Buffer
	err = handleModRunPull([]string{"my-run"}, &buf)
	if err == nil {
		t.Error("handleModRunPull() outside git repo should return error")
	}
	if !strings.Contains(err.Error(), "must be run inside a git repository") {
		t.Errorf("error should mention 'must be run inside a git repository', got: %v", err)
	}
}

// TestHandleModRunPull_DirtyWorkingTree verifies that handleModRunPull fails
// when the working tree has uncommitted changes.
func TestHandleModRunPull_DirtyWorkingTree(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with uncommitted changes.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

	// Add an untracked file to make the working tree dirty.
	untrackedFile := filepath.Join(repoDir, "dirty.txt")
	if err := os.WriteFile(untrackedFile, []byte("dirty content\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	var buf bytes.Buffer
	err = handleModRunPull([]string{"my-run"}, &buf)
	if err == nil {
		t.Error("handleModRunPull() with dirty working tree should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// TestHandleModRunPull_MissingRemote verifies that handleModRunPull fails
// when the specified remote does not exist.
func TestHandleModRunPull_MissingRemote(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with only "origin" remote.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	var buf bytes.Buffer
	// Request a remote that doesn't exist.
	err = handleModRunPull([]string{"--origin", "nonexistent", "my-run"}, &buf)
	if err == nil {
		t.Error("handleModRunPull() with missing remote should return error")
	}
	if !strings.Contains(err.Error(), `git remote "nonexistent" not found`) {
		t.Errorf("error should mention 'git remote \"nonexistent\" not found', got: %v", err)
	}
}

// TestHandleModRunPull_ValidPreconditions verifies that handleModRunPull
// passes all git preconditions and attempts to contact the control plane.
// This test validates that git worktree validation succeeds before API calls.
// Note: Since no control plane is available in unit tests, this test expects
// an API-related error, confirming that git preconditions passed.
func TestHandleModRunPull_ValidPreconditions(t *testing.T) {
	// Skip: This test requires a running control plane to fully validate.
	// Git precondition tests are covered by the individual ensureInsideGitWorktree,
	// ensureCleanWorkingTree, and resolveGitRemoteURL tests.
	t.Skip("requires running control plane; git preconditions validated by dedicated tests")
}

// =============================================================================
// Test Helpers
// =============================================================================

// setupTestGitRepoForPull creates a minimal git repository for testing.
// The repository contains an initial commit with a README.md file.
// Uses t.TempDir() for automatic cleanup.
func setupTestGitRepoForPull(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()

	// Initialize git repo.
	runGitCmdForPull(t, repoDir, "init")
	// Ensure the default branch is 'main' for deterministic tests.
	runGitCmdForPull(t, repoDir, "checkout", "-b", "main")
	runGitCmdForPull(t, repoDir, "config", "user.name", "Test User")
	runGitCmdForPull(t, repoDir, "config", "user.email", "test@example.com")

	// Create initial commit on main.
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runGitCmdForPull(t, repoDir, "add", "README.md")
	runGitCmdForPull(t, repoDir, "commit", "-m", "Initial commit")

	return repoDir
}

// setupTestGitRepoWithRemote creates a git repository with a configured remote.
// This is useful for testing remote resolution logic.
func setupTestGitRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()

	repoDir := setupTestGitRepoForPull(t)

	// Add the "origin" remote with the specified URL.
	runGitCmdForPull(t, repoDir, "remote", "add", "origin", remoteURL)

	return repoDir
}

// runGitCmdForPull executes a git command in the specified directory.
// This helper is used by test fixture setup functions.
func runGitCmdForPull(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (output: %s)", args, err, string(output))
	}
}

// =============================================================================
// Run Resolution Unit Tests (mods.ResolveRunForRepo)
// =============================================================================

// Note: The comprehensive tests for ResolveRunForRepo are in
// internal/cli/mods/repos_test.go. These tests verify integration with
// the mod_run_pull handler and error message formatting.

// =============================================================================
// Branch Collision Detection Tests
// =============================================================================

// TestCheckBranchCollision_LocalBranchExists verifies that checkBranchCollision
// returns an error when a local branch with the target name already exists.
func TestCheckBranchCollision_LocalBranchExists(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with a branch named "feature-branch".
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")
	runGitCmdForPull(t, repoDir, "checkout", "-b", "feature-branch")
	runGitCmdForPull(t, repoDir, "checkout", "main") // Switch back to main

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err = checkBranchCollision(ctx, "origin", "feature-branch", &buf)
	if err == nil {
		t.Error("checkBranchCollision() should return error when local branch exists")
	}
	if !strings.Contains(err.Error(), `branch "feature-branch" already exists locally`) {
		t.Errorf("error should mention local branch exists, got: %v", err)
	}
}

// TestCheckBranchCollision_NoBranchExists verifies that checkBranchCollision
// returns nil when no local or remote branch with the target name exists.
func TestCheckBranchCollision_NoBranchExists(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a clean git repository.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	// Check for a branch that doesn't exist locally.
	// Note: The remote check will fail/succeed based on network, but we're testing local.
	err = checkBranchCollision(ctx, "origin", "nonexistent-branch-xyz", &buf)
	// This test only verifies local branch detection; remote check may vary.
	// The function should not error for local non-existence.
	if err != nil && strings.Contains(err.Error(), "already exists locally") {
		t.Errorf("unexpected local branch collision error: %v", err)
	}
}

// TestCreateAndCheckoutBranch_Success verifies that createAndCheckoutBranch
// successfully creates a new branch at a given commit SHA.
func TestCreateAndCheckoutBranch_Success(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Get the current commit SHA.
	commitSHA := getHeadCommitSHA(t, repoDir)

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err = createAndCheckoutBranch(ctx, "test-new-branch", commitSHA, &buf)
	if err != nil {
		t.Fatalf("createAndCheckoutBranch() should succeed: %v", err)
	}

	// Verify we're on the new branch.
	currentBranch := getCurrentBranch(t, repoDir)
	if currentBranch != "test-new-branch" {
		t.Errorf("expected to be on branch 'test-new-branch', got %q", currentBranch)
	}

	// Verify the output mentions the branch name.
	output := buf.String()
	if !strings.Contains(output, "test-new-branch") {
		t.Errorf("output should mention the branch name, got: %s", output)
	}
}

// TestApplyPatch_Success verifies that applyPatch successfully applies a valid patch.
func TestApplyPatch_Success(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	// Create a simple patch that adds a line to README.md.
	patch := []byte(`diff --git a/README.md b/README.md
index abc1234..def5678 100644
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
 # Test Repo
+Added by patch.
`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = applyPatch(ctx, patch)
	if err != nil {
		t.Fatalf("applyPatch() should succeed: %v", err)
	}

	// Verify the patch was applied by checking file contents.
	content, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	if !strings.Contains(string(content), "Added by patch.") {
		t.Errorf("patch was not applied correctly, got: %s", string(content))
	}
}

// TestApplyPatch_EmptyPatch verifies behavior with empty patches.
// Note: git apply returns an error for truly empty input (no valid patches).
// This is why downloadAndApplyDiffs skips empty patches before calling applyPatch.
func TestApplyPatch_EmptyPatch(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := setupTestGitRepoForPull(t)

	// Change to the repo directory.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change to repo directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Empty patch returns an error from git apply (no valid patches in input).
	// The downloadAndApplyDiffs function skips empty patches before calling applyPatch.
	err = applyPatch(ctx, []byte{})
	if err == nil {
		t.Log("applyPatch() with empty patch did not return error (unexpected but acceptable)")
		return
	}
	// Verify the error mentions the expected git error.
	if !strings.Contains(err.Error(), "git apply failed") {
		t.Errorf("expected git apply error, got: %v", err)
	}
}

// =============================================================================
// Additional Test Helpers
// =============================================================================

// getHeadCommitSHA returns the SHA of the HEAD commit in the given repository.
func getHeadCommitSHA(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get HEAD commit SHA: %v (output: %s)", err, string(output))
	}
	return strings.TrimSpace(string(output))
}

// getCurrentBranch returns the current branch name in the given repository.
func getCurrentBranch(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get current branch: %v (output: %s)", err, string(output))
	}
	return strings.TrimSpace(string(output))
}
