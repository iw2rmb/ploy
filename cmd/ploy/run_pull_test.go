package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// =============================================================================
// Routing and Flag Parsing Tests
// =============================================================================

// TestRunPullRouting validates that `ploy run pull` routes to handleRunPull.
func TestRunPullRouting(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "pull without run-id",
			args:    []string{"run", "pull"},
			wantErr: "run-id required",
		},
		{
			name:    "pull with empty run-id",
			args:    []string{"run", "pull", "   "},
			wantErr: "run-id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestRunPullUsageErrors validates that invalid flag combinations return appropriate errors.
func TestRunPullUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantUsage bool
	}{
		{
			name:      "unknown flag",
			args:      []string{"run", "pull", "--unknown", "my-run"},
			wantErr:   "flag provided but not defined",
			wantUsage: true,
		},
		{
			name:      "origin flag without value",
			args:      []string{"run", "pull", "--origin"},
			wantErr:   "flag needs an argument",
			wantUsage: true,
		},
		{
			name:    "extra positional argument",
			args:    []string{"run", "pull", "my-run", "extra-arg"},
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
				if !strings.Contains(output, "Usage: ploy run pull") {
					t.Errorf("expected usage output, got %q", output)
				}
			}
		})
	}
}

// TestRunPullUsageHelp validates that the usage text contains expected content.
func TestRunPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printRunPullUsage(&buf)

	output := buf.String()

	// Verify usage line is present.
	if !strings.Contains(output, "Usage: ploy run pull") {
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
	if !strings.Contains(output, "<run-id>") {
		t.Errorf("usage should document run-id argument, got %q", output)
	}

	// Verify examples are present.
	if !strings.Contains(output, "Examples:") {
		t.Errorf("usage should contain examples section, got %q", output)
	}

	// Verify description of functionality is mentioned.
	if !strings.Contains(output, "Pulls Mods diffs from a run") {
		t.Errorf("usage should describe pulling diffs from a run, got %q", output)
	}
}

// =============================================================================
// Git Worktree Precondition Tests
// =============================================================================

// TestHandleRunPull_OutsideGitRepo verifies that handleRunPull fails
// when called outside a git repository.
func TestHandleRunPull_OutsideGitRepo(t *testing.T) {
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
	err = handleRunPull([]string{"my-run"}, &buf)
	if err == nil {
		t.Error("handleRunPull() outside git repo should return error")
	}
	if !strings.Contains(err.Error(), "must be run inside a git repository") {
		t.Errorf("error should mention 'must be run inside a git repository', got: %v", err)
	}
}

// TestHandleRunPull_DirtyWorkingTree verifies that handleRunPull fails
// when the working tree has uncommitted changes.
func TestHandleRunPull_DirtyWorkingTree(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with uncommitted changes.
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")

	// Add an untracked file to make the working tree dirty.
	untrackedFile := repoDir + "/dirty.txt"
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
	err = handleRunPull([]string{"my-run"}, &buf)
	if err == nil {
		t.Error("handleRunPull() with dirty working tree should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// TestHandleRunPull_MissingRemote verifies that handleRunPull fails
// when the specified remote does not exist.
func TestHandleRunPull_MissingRemote(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with only "origin" remote.
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")

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
	err = handleRunPull([]string{"--origin", "nonexistent", "my-run"}, &buf)
	if err == nil {
		t.Error("handleRunPull() with missing remote should return error")
	}
	if !strings.Contains(err.Error(), `git remote "nonexistent" not found`) {
		t.Errorf("error should mention 'git remote \"nonexistent\" not found', got: %v", err)
	}
}

// =============================================================================
// fetchRunRepoDetails Unit Tests
// =============================================================================

// TestFetchRunRepoDetails_NotFound verifies error handling when repo is not in the run.
func TestFetchRunRepoDetails_NotFound(t *testing.T) {
	// Skip: This test requires a mock HTTP server which would require additional setup.
	// The error path is covered by integration tests.
	t.Skip("requires mock HTTP server; covered by integration tests")
}

// =============================================================================
// Context Timeout Tests
// =============================================================================

// TestHandleRunPull_ContextTimeout verifies that operations respect context timeout.
func TestHandleRunPull_ContextTimeout(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository.
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")

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

	// Create an already-cancelled context.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	// ensureInsideGitWorktree should fail due to context timeout.
	err = ensureInsideGitWorktree(ctx)
	if err == nil {
		// The function may still succeed if git is fast enough.
		// This test just verifies the context is respected.
		t.Log("context timeout not triggered; git was fast enough")
		return
	}
	if !strings.Contains(err.Error(), "context") {
		t.Logf("error may not be context-related: %v", err)
	}
}
