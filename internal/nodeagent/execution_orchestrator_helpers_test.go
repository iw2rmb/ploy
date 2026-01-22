package nodeagent

import (
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestWithTempDir_CreatesAndCleansUp verifies that withTempDir creates a temp directory,
// calls the provided function, and cleans up the directory afterward.
func TestWithTempDir_CreatesAndCleansUp(t *testing.T) {
	var capturedDir string

	// Verify the directory is passed to the function and is valid.
	err := withTempDir("test-prefix-*", func(dir string) error {
		capturedDir = dir
		// Verify the directory exists.
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Fatalf("temp directory does not exist: %s", dir)
		}
		// Create a file in the directory to verify cleanup.
		testFile := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("withTempDir returned error: %v", err)
	}

	// Verify the directory was cleaned up.
	if _, err := os.Stat(capturedDir); !os.IsNotExist(err) {
		t.Fatalf("temp directory was not cleaned up: %s", capturedDir)
	}
}

// TestWithTempDir_CleansUpOnError verifies that withTempDir cleans up the directory
// even when the function returns an error.
func TestWithTempDir_CleansUpOnError(t *testing.T) {
	var capturedDir string

	err := withTempDir("test-error-*", func(dir string) error {
		capturedDir = dir
		// Return an error to simulate failure.
		return os.ErrInvalid
	})

	// Verify error is propagated.
	if err != os.ErrInvalid {
		t.Fatalf("expected os.ErrInvalid, got: %v", err)
	}

	// Verify the directory was still cleaned up.
	if _, err := os.Stat(capturedDir); !os.IsNotExist(err) {
		t.Fatalf("temp directory was not cleaned up after error: %s", capturedDir)
	}
}

// TestSnapshotWorkspaceForNoIndexDiff_CreatesSnapshot verifies that the snapshot helper
// creates a copy of the workspace for baseline comparison.
func TestSnapshotWorkspaceForNoIndexDiff_CreatesSnapshot(t *testing.T) {
	// Create a test workspace with a git repo.
	workspace := t.TempDir()
	gitDir := filepath.Join(workspace, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git directory: %v", err)
	}

	// Create a test file in the workspace.
	testFile := filepath.Join(workspace, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	runID := types.RunID("test-run")
	jobID := types.JobID("test-job")

	result := snapshotWorkspaceForNoIndexDiff(runID, jobID, DiffModTypeMod, workspace)
	defer result.cleanup()

	// Verify the snapshot was created.
	if result.dir == "" {
		t.Fatal("snapshot directory is empty")
	}

	// Verify the snapshot contains the test file.
	copiedFile := filepath.Join(result.dir, "test.txt")
	data, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("copied file content mismatch: got %q, want %q", string(data), "content")
	}
}

// TestSnapshotWorkspaceForNoIndexDiff_CleanupWorks verifies that the cleanup function
// removes the snapshot directory.
func TestSnapshotWorkspaceForNoIndexDiff_CleanupWorks(t *testing.T) {
	// Create a test workspace with a git repo.
	workspace := t.TempDir()
	gitDir := filepath.Join(workspace, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git directory: %v", err)
	}

	runID := types.RunID("test-run")
	jobID := types.JobID("test-job")

	result := snapshotWorkspaceForNoIndexDiff(runID, jobID, DiffModTypeHealing, workspace)

	// Capture the snapshot directory path.
	snapshotDir := result.dir
	if snapshotDir == "" {
		t.Fatal("snapshot directory is empty")
	}

	// Call cleanup.
	result.cleanup()

	// Verify the snapshot was cleaned up.
	if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
		t.Fatalf("snapshot directory was not cleaned up: %s", snapshotDir)
	}
}

// TestSnapshotWorkspaceForNoIndexDiff_NoGitRepo_ReturnsEmpty verifies that when
// the workspace is not a git repo, the snapshot returns an empty dir with safe cleanup.
func TestSnapshotWorkspaceForNoIndexDiff_NoGitRepo_ReturnsEmpty(t *testing.T) {
	// Create a workspace without .git directory.
	workspace := t.TempDir()

	runID := types.RunID("test-run")
	jobID := types.JobID("test-job")

	result := snapshotWorkspaceForNoIndexDiff(runID, jobID, DiffModTypeMod, workspace)
	defer result.cleanup() // Should be safe to call even with empty dir.

	// Verify the snapshot failed (empty dir).
	if result.dir != "" {
		t.Fatalf("expected empty dir for non-git workspace, got: %s", result.dir)
	}
}

// TestClearManifestHydration_RemovesHydration verifies that clearManifestHydration
// sets Hydration to nil on all inputs.
func TestClearManifestHydration_RemovesHydration(t *testing.T) {
	manifest := contracts.StepManifest{
		Inputs: []contracts.StepInput{
			{
				Name:      "input1",
				Hydration: &contracts.StepInputHydration{},
			},
			{
				Name:      "input2",
				Hydration: &contracts.StepInputHydration{},
			},
		},
	}

	clearManifestHydration(&manifest)

	for i, input := range manifest.Inputs {
		if input.Hydration != nil {
			t.Errorf("input[%d].Hydration should be nil, got: %v", i, input.Hydration)
		}
	}
}

// TestClearManifestHydration_EmptyInputs_NoOp verifies that clearManifestHydration
// handles empty inputs gracefully.
func TestClearManifestHydration_EmptyInputs_NoOp(t *testing.T) {
	manifest := contracts.StepManifest{
		Inputs: []contracts.StepInput{},
	}

	// Should not panic or modify anything.
	clearManifestHydration(&manifest)

	if len(manifest.Inputs) != 0 {
		t.Errorf("expected empty inputs, got: %d", len(manifest.Inputs))
	}
}

// TestDisableManifestGate_SetsGateDisabled verifies that disableManifestGate
// sets Gate.Enabled to false.
func TestDisableManifestGate_SetsGateDisabled(t *testing.T) {
	manifest := contracts.StepManifest{
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	disableManifestGate(&manifest)

	if manifest.Gate == nil {
		t.Fatal("Gate should not be nil")
	}
	if manifest.Gate.Enabled {
		t.Error("Gate.Enabled should be false")
	}
}

// TestDisableManifestGate_NilGate_SetsDisabledGate verifies that disableManifestGate
// handles nil Gate by setting a disabled gate.
func TestDisableManifestGate_NilGate_SetsDisabledGate(t *testing.T) {
	manifest := contracts.StepManifest{
		Gate: nil,
	}

	disableManifestGate(&manifest)

	if manifest.Gate == nil {
		t.Fatal("Gate should not be nil after disableManifestGate")
	}
	if manifest.Gate.Enabled {
		t.Error("Gate.Enabled should be false")
	}
}

// TestWorkspaceRehydrationResult_CleanupRemovesDirectory verifies that the cleanup
// function in workspaceRehydrationResult removes the workspace.
func TestWorkspaceRehydrationResult_CleanupRemovesDirectory(t *testing.T) {
	// Create a temp directory to simulate a workspace.
	workspace := t.TempDir()
	testFile := filepath.Join(workspace, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a workspaceRehydrationResult with the cleanup function.
	result := workspaceRehydrationResult{
		workspace: workspace,
		cleanup:   func() { _ = os.RemoveAll(workspace) },
	}

	// Call cleanup.
	result.cleanup()

	// Verify the workspace was removed.
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace was not cleaned up: %s", workspace)
	}
}

// TestSnapshotResult_CleanupIsSafeWhenEmpty verifies that snapshotResult.cleanup()
// is safe to call even when dir is empty.
func TestSnapshotResult_CleanupIsSafeWhenEmpty(t *testing.T) {
	result := snapshotResult{
		dir:     "",
		cleanup: func() {}, // No-op cleanup for empty result.
	}

	// Should not panic.
	result.cleanup()
}
