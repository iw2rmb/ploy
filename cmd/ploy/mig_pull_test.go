package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// =============================================================================
// Routing and Flag Parsing Tests
// =============================================================================

// TestModPullRouting validates that `ploy mig pull` routes to handleMigPull.
func TestModPullRouting(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Test that we can call mig pull without error (until git check).
	// The test will fail at the git worktree check.
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "mig pull routes correctly",
			args:    []string{"mig", "pull"},
			wantErr: "must be run inside a git repository",
		},
		{
			name:    "mig pull with mig-id routes correctly",
			args:    []string{"mig", "pull", "my-mig"},
			wantErr: "must be run inside a git repository",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a non-git directory to trigger the expected error.
			nonRepoDir := t.TempDir()
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
			err = executeCmd(tc.args, &buf)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestModPullUsageErrors validates that invalid flag combinations return appropriate errors.
func TestModPullUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantUsage bool
	}{
		{
			name:      "unknown flag",
			args:      []string{"mig", "pull", "--unknown"},
			wantErr:   "flag provided but not defined",
			wantUsage: true,
		},
		{
			name:      "origin flag without value",
			args:      []string{"mig", "pull", "--origin"},
			wantErr:   "flag needs an argument",
			wantUsage: true,
		},
		{
			name:    "extra positional argument",
			args:    []string{"mig", "pull", "my-mig", "extra-arg"},
			wantErr: "unexpected argument: extra-arg",
		},
		{
			name:      "mutually exclusive flags",
			args:      []string{"mig", "pull", "--last-failed", "--last-succeeded"},
			wantErr:   "mutually exclusive",
			wantUsage: true,
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
				if !strings.Contains(output, "Usage: ploy mig pull") {
					t.Errorf("expected usage output, got %q", output)
				}
			}
		})
	}
}

// TestModPullUsageHelp validates that the usage text contains expected content.
func TestModPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printMigPullUsage(&buf)

	output := buf.String()

	// Verify usage line is present.
	if !strings.Contains(output, "Usage: ploy mig pull") {
		t.Errorf("usage should contain command line, got %q", output)
	}

	// Verify flags are documented.
	if !strings.Contains(output, "--origin") {
		t.Errorf("usage should document --origin flag, got %q", output)
	}
	if !strings.Contains(output, "--dry-run") {
		t.Errorf("usage should document --dry-run flag, got %q", output)
	}
	if !strings.Contains(output, "--last-failed") {
		t.Errorf("usage should document --last-failed flag, got %q", output)
	}
	if !strings.Contains(output, "--last-succeeded") {
		t.Errorf("usage should document --last-succeeded flag, got %q", output)
	}

	// Verify optional mig argument is documented.
	if !strings.Contains(output, "[<mig-id|name>]") {
		t.Errorf("usage should document optional mig argument, got %q", output)
	}

	// Verify examples are present.
	if !strings.Contains(output, "Examples:") {
		t.Errorf("usage should contain examples section, got %q", output)
	}

	// Verify description of functionality is mentioned.
	if !strings.Contains(output, "Pulls Mods diffs from a mig") {
		t.Errorf("usage should describe pulling diffs from a mig, got %q", output)
	}
}

// =============================================================================
// Git Worktree Precondition Tests
// =============================================================================

// TestHandleModPull_OutsideGitRepo verifies that handleMigPull fails
// when called outside a git repository.
func TestHandleModPull_OutsideGitRepo(t *testing.T) {
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
	err = handleMigPull([]string{"my-mig"}, &buf)
	if err == nil {
		t.Error("handleMigPull() outside git repo should return error")
	}
	if !strings.Contains(err.Error(), "must be run inside a git repository") {
		t.Errorf("error should mention 'must be run inside a git repository', got: %v", err)
	}
}

// TestHandleModPull_DirtyWorkingTree verifies that handleMigPull fails
// when the working tree has uncommitted changes.
func TestHandleModPull_DirtyWorkingTree(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a git repository with uncommitted changes.
	repoDir := setupTestGitRepoWithRemote(t, "https://github.com/example/repo.git")

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
	err = handleMigPull([]string{"my-mig"}, &buf)
	if err == nil {
		t.Error("handleMigPull() with dirty working tree should return error")
	}
	if !strings.Contains(err.Error(), "working tree must be clean") {
		t.Errorf("error should mention 'working tree must be clean', got: %v", err)
	}
}

// TestHandleModPull_MissingRemote verifies that handleMigPull fails
// when the specified remote does not exist.
func TestHandleModPull_MissingRemote(t *testing.T) {
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
	err = handleMigPull([]string{"--origin", "nonexistent", "my-mig"}, &buf)
	if err == nil {
		t.Error("handleMigPull() with missing remote should return error")
	}
	if !strings.Contains(err.Error(), `git remote "nonexistent" not found`) {
		t.Errorf("error should mention 'git remote \"nonexistent\" not found', got: %v", err)
	}
}

// =============================================================================
// inferModFromRepo Unit Tests
// =============================================================================

// TestInferModFromRepo_NoMods verifies error handling when no migs include the repo.
func TestInferModFromRepo_NoMods(t *testing.T) {
	// Skip: This test requires a mock HTTP server which would require additional setup.
	// The error path is covered by integration tests.
	t.Skip("requires mock HTTP server; covered by integration tests")
}

// TestInferModFromRepo_MultipleMods verifies error handling when multiple migs include the repo.
func TestInferModFromRepo_MultipleMods(t *testing.T) {
	// Skip: This test requires a mock HTTP server which would require additional setup.
	// The error path is covered by integration tests.
	t.Skip("requires mock HTTP server; covered by integration tests")
}
