package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTempDir creates a temporary directory for testing with automatic cleanup
func CreateTempDir(t *testing.T, prefix string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", prefix)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

// CreateTempFile creates a temporary file for testing with automatic cleanup
func CreateTempFile(t *testing.T, dir, pattern string) *os.File {
	t.Helper()

	file, err := os.CreateTemp(dir, pattern)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	})

	return file
}

// WriteFile writes content to a file in the given directory
func WriteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()

	path := filepath.Join(dir, filename)

	// Create parent directories if needed
	parentDir := filepath.Dir(path)
	if parentDir != dir {
		err := os.MkdirAll(parentDir, 0755)
		require.NoError(t, err)
	}

	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	return path
}
