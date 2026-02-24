// Package integration provides integration tests for the Ploy platform.
// This file contains build verification tests to ensure CLI binaries
// are correctly produced by the build system.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const goModuleFile = "go." + "mo" + "d"

// TestBuildCLI verifies that `make build` produces the dist/ploy binary.
// This test ensures the build system is working correctly and that the
// CLI entrypoint can be compiled successfully.
func TestBuildCLI(t *testing.T) {
	// Find the repository root by walking up from the test directory.
	// This allows the test to run from any working directory.
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to find repository root: %v", err)
	}

	// Define the expected binary path relative to repo root.
	binaryPath := filepath.Join(repoRoot, "dist", "ploy")

	// Clean the dist directory to ensure we're testing a fresh build.
	// This prevents false positives from stale binaries.
	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = repoRoot
	if err := cleanCmd.Run(); err != nil {
		t.Logf("warning: make clean failed (may not be critical): %v", err)
	}

	// Run `make build` to compile the CLI binary.
	// We set the working directory to the repo root to ensure
	// the Makefile is found correctly.
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = repoRoot
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make build failed: %v\nOutput: %s", err, output)
	}

	// Verify the binary exists at the expected path.
	// This confirms that the build target created the file
	// in the correct location.
	if _, err := os.Stat(binaryPath); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("binary not found at %s after make build", binaryPath)
		}
		t.Fatalf("failed to stat binary at %s: %v", binaryPath, err)
	}

	// Verify the binary is executable.
	// The build system should set appropriate permissions.
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("failed to stat binary: %v", err)
	}
	mode := info.Mode()
	if mode&0111 == 0 {
		t.Fatalf("binary at %s is not executable (mode: %v)", binaryPath, mode)
	}

	// Optionally verify the binary runs and prints version info.
	// This is a basic smoke test to ensure the binary is functional.
	versionCmd := exec.Command(binaryPath, "version")
	versionOutput, err := versionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run ploy version: %v\nOutput: %s", err, versionOutput)
	}

	t.Logf("Build successful: binary exists at %s and is executable", binaryPath)
	t.Logf("Version output: %s", versionOutput)
}

// findRepoRoot walks up the directory tree from the current working directory
// to find the repository root (identified by the Go module file).
func findRepoRoot() (string, error) {
	// Start from the current working directory.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree until we find the Go module file.
	// This indicates the repository root.
	for {
		goModPath := filepath.Join(dir, goModuleFile)
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		// Move up one directory level.
		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the filesystem root without finding the Go module file.
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
