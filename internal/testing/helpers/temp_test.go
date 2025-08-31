package helpers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

func TestCreateTempDir(t *testing.T) {
	t.Run("creates temporary directory", func(t *testing.T) {
		dir := helpers.CreateTempDir(t, "test-prefix")
		
		// Verify directory exists
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		
		// Verify prefix is in the name
		assert.Contains(t, dir, "test-prefix")
	})
	
	t.Run("cleanup removes directory", func(t *testing.T) {
		var dirPath string
		
		// Create a sub-test to trigger cleanup
		t.Run("inner", func(t *testing.T) {
			dirPath = helpers.CreateTempDir(t, "cleanup-test")
			
			// Verify it exists
			_, err := os.Stat(dirPath)
			require.NoError(t, err)
		})
		
		// After inner test cleanup, directory should not exist
		_, err := os.Stat(dirPath)
		assert.True(t, os.IsNotExist(err), "Directory should be removed after test cleanup")
	})
}

func TestCreateTempFile(t *testing.T) {
	t.Run("creates temporary file", func(t *testing.T) {
		dir := t.TempDir()
		file := helpers.CreateTempFile(t, dir, "test-file-*.txt")
		
		// Verify file exists
		info, err := os.Stat(file.Name())
		require.NoError(t, err)
		assert.False(t, info.IsDir())
		
		// Verify pattern is in the name
		assert.Contains(t, filepath.Base(file.Name()), "test-file-")
		assert.Contains(t, file.Name(), ".txt")
		
		// Verify we can write to it
		_, err = file.WriteString("test content")
		assert.NoError(t, err)
	})
	
	t.Run("cleanup removes file", func(t *testing.T) {
		dir := t.TempDir()
		var fileName string
		
		// Create a sub-test to trigger cleanup
		t.Run("inner", func(t *testing.T) {
			file := helpers.CreateTempFile(t, dir, "cleanup-*.txt")
			fileName = file.Name()
			
			// Verify it exists
			_, err := os.Stat(fileName)
			require.NoError(t, err)
		})
		
		// After inner test cleanup, file should not exist
		_, err := os.Stat(fileName)
		assert.True(t, os.IsNotExist(err), "File should be removed after test cleanup")
	})
}

func TestWriteFile(t *testing.T) {
	t.Run("writes content to file", func(t *testing.T) {
		dir := t.TempDir()
		content := "test content\nwith multiple lines\n"
		
		path := helpers.WriteFile(t, dir, "test.txt", content)
		
		// Verify file exists
		assert.Equal(t, filepath.Join(dir, "test.txt"), path)
		
		// Verify content
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	})
	
	t.Run("creates file with correct permissions", func(t *testing.T) {
		dir := t.TempDir()
		
		path := helpers.WriteFile(t, dir, "perms.txt", "content")
		
		info, err := os.Stat(path)
		require.NoError(t, err)
		
		// Check permissions (0644)
		mode := info.Mode()
		assert.Equal(t, os.FileMode(0644), mode.Perm())
	})
	
	t.Run("handles nested paths", func(t *testing.T) {
		dir := t.TempDir()
		
		path := helpers.WriteFile(t, dir, "subdir/nested.txt", "nested content")
		
		expectedPath := filepath.Join(dir, "subdir/nested.txt")
		assert.Equal(t, expectedPath, path)
		
		// Verify file exists and has content
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(data))
	})
}