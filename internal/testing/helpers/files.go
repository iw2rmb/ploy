package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CreateTempDirLegacy creates a temporary directory for testing (legacy signature)
func CreateTempDirLegacy(t testing.TB) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "ploy-test-")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})

	return tmpDir
}

// CleanupTempDir removes a temporary directory (deprecated - use CreateTempDir with t.Cleanup)
func CleanupTempDir(t testing.TB, path string) {
	t.Helper()
	err := os.RemoveAll(path)
	if err != nil {
		t.Logf("Failed to cleanup temp dir %s: %v", path, err)
	}
}

// CreateTestFiles creates test files in a directory
func CreateTestFiles(t testing.TB, baseDir string, files map[string]string) {
	t.Helper()

	for relPath, content := range files {
		fullPath := filepath.Join(baseDir, relPath)

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if dir != baseDir {
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
		}

		// Write file
		err := os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}
}

// CreateTestFilesFromSlice creates test files from a slice of filenames
func CreateTestFilesFromSlice(t testing.TB, baseDir string, filenames []string) {
	t.Helper()

	files := make(map[string]string)
	for _, filename := range filenames {
		files[filename] = "# Test file: " + filename
	}

	CreateTestFiles(t, baseDir, files)
}

// WriteTestFile writes a single test file
func WriteTestFile(t testing.TB, path, content string) {
	t.Helper()

	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

// ReadTestFile reads a test file and returns its contents
func ReadTestFile(t testing.TB, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(content)
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists checks if a directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// AssertFileExists asserts that a file exists
func AssertFileExists(t testing.TB, path string) {
	t.Helper()
	assert.True(t, FileExists(path), "File should exist: %s", path)
}

// AssertFileNotExists asserts that a file does not exist
func AssertFileNotExists(t testing.TB, path string) {
	t.Helper()
	assert.False(t, FileExists(path), "File should not exist: %s", path)
}

// AssertDirExists asserts that a directory exists
func AssertDirExists(t testing.TB, path string) {
	t.Helper()
	assert.True(t, DirExists(path), "Directory should exist: %s", path)
}

// AssertFileContains asserts that a file contains the specified content
func AssertFileContains(t testing.TB, path, expectedContent string) {
	t.Helper()

	content := ReadTestFile(t, path)
	assert.Contains(t, content, expectedContent, "File should contain expected content")
}
