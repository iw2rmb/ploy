package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	tempDir := t.TempDir()
	
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	assert.NotNil(t, manager)
	assert.Equal(t, tempDir, manager.workDir)
	assert.Equal(t, "testuser", manager.runAsUser)
	assert.Equal(t, "512MB", manager.maxMemory)
	assert.Equal(t, "1.0", manager.maxCPU)
}

func TestManager_ExtractArchive_TarGz(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	// Create test tar.gz archive
	archiveData := createTestTarGzArchive(t, map[string]string{
		"test.py":        "print('Hello World')",
		"subdir/test2.py": "import os",
	})
	
	extractPath, cleanup, err := manager.ExtractArchive(context.Background(), archiveData)
	require.NoError(t, err)
	defer cleanup()
	
	// Verify extracted files
	assert.FileExists(t, filepath.Join(extractPath, "test.py"))
	assert.FileExists(t, filepath.Join(extractPath, "subdir", "test2.py"))
	
	// Verify file contents
	content, err := os.ReadFile(filepath.Join(extractPath, "test.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('Hello World')", string(content))
}

func TestManager_ExtractArchive_InvalidFormat(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	invalidData := []byte("invalid archive data")
	
	_, _, err := manager.ExtractArchive(context.Background(), invalidData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create gzip reader")
}

func TestManager_ExtractArchive_EmptyArchive(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	// Create empty tar.gz archive
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	
	err := tarWriter.Close()
	require.NoError(t, err)
	err = gzWriter.Close()
	require.NoError(t, err)
	
	extractPath, cleanup, err := manager.ExtractArchive(context.Background(), buf.Bytes())
	require.NoError(t, err)
	defer cleanup()
	
	// Verify directory exists but is empty
	entries, err := os.ReadDir(extractPath)
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestManager_ExecuteCommand(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	// Test with simple echo command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	result, err := manager.ExecuteCommand(ctx, "echo", []string{"hello world"}, tempDir)
	require.NoError(t, err)
	
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello world\n", result.Stdout)
	assert.Empty(t, result.Stderr)
}

func TestManager_ExecuteCommand_WithTimeout(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	// Test command that should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	result, err := manager.ExecuteCommand(ctx, "sleep", []string{"1"}, tempDir)
	
	// Command should be killed due to timeout or return with non-zero exit
	if err != nil {
		assert.Contains(t, err.Error(), "context deadline exceeded")
	} else {
		// If no error, the command should have non-zero exit code due to being killed
		assert.NotNil(t, result)
		assert.NotEqual(t, 0, result.ExitCode)
	}
}

func TestManager_ExecuteCommand_NonExistentCommand(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	result, err := manager.ExecuteCommand(ctx, "nonexistent-command", []string{}, tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start command")
	assert.Nil(t, result)
}

func TestManager_ExecuteCommand_CommandFailure(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Command that will fail with non-zero exit code
	result, err := manager.ExecuteCommand(ctx, "ls", []string{"/nonexistent-directory"}, tempDir)
	
	// Command should complete but with non-zero exit code
	require.NoError(t, err) // No execution error, just command failure
	assert.NotEqual(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Stderr)
}

func TestManager_ValidateArchive(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	tests := []struct {
		name         string
		files        map[string]string
		allowedExts  []string
		maxSizeBytes int64
		wantErr      bool
		errMsg       string
	}{
		{
			name: "valid python files",
			files: map[string]string{
				"test.py":  "print('hello')",
				"main.pyw": "# Python file",
			},
			allowedExts:  []string{".py", ".pyw"},
			maxSizeBytes: 1024,
			wantErr:      false,
		},
		{
			name: "invalid file extension",
			files: map[string]string{
				"test.js": "console.log('hello')",
			},
			allowedExts:  []string{".py", ".pyw"},
			maxSizeBytes: 1024,
			wantErr:      true,
			errMsg:       "not allowed",
		},
		{
			name: "archive too large",
			files: map[string]string{
				"test.py": strings.Repeat("a", 1000),
			},
			allowedExts:  []string{".py"},
			maxSizeBytes: 100,
			wantErr:      true,
			errMsg:       "archive exceeds maximum size",
		},
		{
			name: "directory traversal attempt",
			files: map[string]string{
				"../../../etc/passwd": "malicious content",
			},
			allowedExts:  []string{".py"},
			maxSizeBytes: 1024,
			wantErr:      true,
			errMsg:       "path traversal detected",
		},
		{
			name: "absolute path attempt",
			files: map[string]string{
				"/etc/hosts": "malicious content",
			},
			allowedExts:  []string{".py"},
			maxSizeBytes: 1024,
			wantErr:      true,
			errMsg:       "absolute path not allowed",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archiveData := createTestTarGzArchive(t, tt.files)
			
			err := manager.ValidateArchive(archiveData, tt.allowedExts, tt.maxSizeBytes)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_CreateWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir, "testuser", "512MB", "1.0")
	
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()
	
	// Verify directory exists and is within tempDir
	assert.DirExists(t, workDir)
	assert.Contains(t, workDir, tempDir)
	
	// Verify cleanup function works
	cleanup()
	assert.NoDirExists(t, workDir)
}

// Helper function to create test tar.gz archives
func createTestTarGzArchive(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	
	for filename, content := range files {
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}
		
		err := tarWriter.WriteHeader(header)
		require.NoError(t, err)
		
		_, err = tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	
	err := tarWriter.Close()
	require.NoError(t, err)
	err = gzWriter.Close()
	require.NoError(t, err)
	
	return buf.Bytes()
}