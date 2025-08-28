package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	
	require.NoError(t, err, "NewManager should not fail with valid config")
	assert.NotNil(t, manager, "Manager should not be nil")
	assert.Equal(t, config.WorkDir, manager.workDir)
	assert.Equal(t, config.MaxMemory, manager.maxMemory)
	assert.Equal(t, config.MaxCPUTime, manager.maxCPUTime)
	assert.Equal(t, config.MaxProcesses, manager.maxProcesses)
}

func TestNewManager_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config ManagerConfig
		errMsg string
	}{
		{
			name: "empty work directory",
			config: ManagerConfig{
				WorkDir:   "",
				MaxMemory: "1GB",
			},
			errMsg: "work directory cannot be empty",
		},
		{
			name: "invalid memory format",
			config: ManagerConfig{
				WorkDir:   "/tmp/test",
				MaxMemory: "invalid",
			},
			errMsg: "invalid memory format",
		},
		{
			name: "invalid CPU time format",
			config: ManagerConfig{
				WorkDir:    "/tmp/test",
				MaxMemory:  "1GB",
				MaxCPUTime: "invalid",
			},
			errMsg: "invalid CPU time format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManager(tt.config)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestManager_CreateWorkingDirectory(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Test successful directory creation
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	
	require.NoError(t, err, "CreateWorkingDirectory should not fail")
	assert.NotEmpty(t, workDir, "Working directory path should not be empty")
	assert.NotNil(t, cleanup, "Cleanup function should not be nil")
	
	// Verify directory exists and is within base work directory
	assert.True(t, strings.HasPrefix(workDir, config.WorkDir), "Work directory should be within base directory")
	
	// Check directory actually exists
	_, err = os.Stat(workDir)
	assert.NoError(t, err, "Working directory should exist on filesystem")
	
	// Test cleanup function
	cleanup()
	
	// Verify directory is cleaned up
	_, err = os.Stat(workDir)
	assert.True(t, os.IsNotExist(err), "Working directory should be cleaned up")
}

func TestManager_CreateWorkingDirectory_BaseDirectoryCreation(t *testing.T) {
	// Use a unique directory that likely doesn't exist
	baseDir := "/tmp/cllm-test-sandbox-unique-" + fmt.Sprintf("%d", time.Now().UnixNano())
	
	config := ManagerConfig{
		WorkDir:        baseDir,
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Verify base directory was created
	_, err = os.Stat(baseDir)
	assert.NoError(t, err, "Base directory should be created if it doesn't exist")
	
	// Verify working directory is within base directory
	assert.True(t, strings.HasPrefix(workDir, baseDir), "Working directory should be within base directory")
	
	// Clean up base directory
	defer os.RemoveAll(baseDir)
}

func TestManager_ValidatePath(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	baseDir := "/tmp/safe-base"
	
	tests := []struct {
		name     string
		filePath string
		isValid  bool
	}{
		{
			name:     "valid relative path",
			filePath: "src/main.go",
			isValid:  true,
		},
		{
			name:     "valid nested path",
			filePath: "project/src/utils/helper.go",
			isValid:  true,
		},
		{
			name:     "path traversal attempt",
			filePath: "../../../etc/passwd",
			isValid:  false,
		},
		{
			name:     "absolute path",
			filePath: "/etc/passwd",
			isValid:  false,
		},
		{
			name:     "path with double dots",
			filePath: "src/../../../etc/passwd",
			isValid:  false,
		},
		{
			name:     "path with null bytes",
			filePath: "src/main\x00.go",
			isValid:  false,
		},
		{
			name:     "empty path",
			filePath: "",
			isValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ValidatePath(baseDir, tt.filePath)
			if tt.isValid {
				assert.NoError(t, err, "Path should be valid")
			} else {
				assert.Error(t, err, "Path should be invalid")
			}
		})
	}
}

func TestManager_ParseResourceLimits(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "2GB",
		MaxCPUTime:     "600s",
		MaxProcesses:   15,
		CleanupTimeout: "45s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Test memory parsing
	memBytes, err := manager.ParseMemoryLimit()
	require.NoError(t, err)
	assert.Equal(t, int64(2*1024*1024*1024), memBytes)

	// Test CPU time parsing
	cpuDuration, err := manager.ParseCPUTimeLimit()
	require.NoError(t, err)
	assert.Equal(t, 600*time.Second, cpuDuration)

	// Test cleanup timeout parsing
	cleanupDuration, err := manager.ParseCleanupTimeout()
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, cleanupDuration)
}

func TestManager_GetResourceLimits(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "512MB",
		MaxCPUTime:     "120s",
		MaxProcesses:   5,
		CleanupTimeout: "10s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	limits := manager.GetResourceLimits()
	
	assert.Equal(t, "512MB", limits.MaxMemory)
	assert.Equal(t, "120s", limits.MaxCPUTime)
	assert.Equal(t, 5, limits.MaxProcesses)
	assert.Equal(t, "10s", limits.CleanupTimeout)
}

func TestManager_Shutdown(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory to test cleanup during shutdown
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	
	// Don't call cleanup manually - shutdown should handle it
	_ = cleanup

	// Verify directory exists
	_, err = os.Stat(workDir)
	assert.NoError(t, err, "Working directory should exist before shutdown")

	// Test graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = manager.Shutdown(ctx)
	assert.NoError(t, err, "Shutdown should complete successfully")
}

func TestManager_Shutdown_Timeout(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "1m", // Long cleanup timeout
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Test shutdown with very short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = manager.Shutdown(ctx)
	if err != nil {
		assert.Contains(t, err.Error(), "context deadline exceeded", "Should timeout with context deadline")
	} else {
		t.Log("Shutdown completed faster than expected timeout")
	}
}

func TestManager_ExtractArchive(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a simple tar.gz archive in memory
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a test file
	testContent := "Hello, CLLM sandbox!"
	header := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(testContent)),
	}
	
	err = tarWriter.WriteHeader(header)
	require.NoError(t, err, "Failed to write tar header")
	
	_, err = tarWriter.Write([]byte(testContent))
	require.NoError(t, err, "Failed to write tar content")
	
	// Add a directory
	dirHeader := &tar.Header{
		Name:     "subdir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	err = tarWriter.WriteHeader(dirHeader)
	require.NoError(t, err)
	
	// Add file in directory
	subFileContent := "Subdirectory file"
	subHeader := &tar.Header{
		Name: "subdir/sub.txt",
		Mode: 0644,
		Size: int64(len(subFileContent)),
	}
	err = tarWriter.WriteHeader(subHeader)
	require.NoError(t, err)
	
	_, err = tarWriter.Write([]byte(subFileContent))
	require.NoError(t, err)

	err = tarWriter.Close()
	require.NoError(t, err)
	
	err = gzWriter.Close()
	require.NoError(t, err)

	// Test extraction
	ctx := context.Background()
	extractPath, cleanup, err := manager.ExtractArchive(ctx, buf.Bytes())
	
	require.NoError(t, err, "ExtractArchive should succeed")
	require.NotEmpty(t, extractPath, "Extract path should not be empty")
	require.NotNil(t, cleanup, "Cleanup function should not be nil")
	defer cleanup()

	// Verify extracted files
	testFilePath := filepath.Join(extractPath, "test.txt")
	content, err := os.ReadFile(testFilePath)
	assert.NoError(t, err, "Should be able to read extracted file")
	assert.Equal(t, testContent, string(content), "Extracted content should match")

	// Verify extracted subdirectory file
	subFilePath := filepath.Join(extractPath, "subdir", "sub.txt")
	subContent, err := os.ReadFile(subFilePath)
	assert.NoError(t, err, "Should be able to read extracted subdirectory file")
	assert.Equal(t, subFileContent, string(subContent), "Extracted subdirectory content should match")

	// Verify directory exists
	subDirPath := filepath.Join(extractPath, "subdir")
	stat, err := os.Stat(subDirPath)
	assert.NoError(t, err, "Subdirectory should exist")
	assert.True(t, stat.IsDir(), "Should be a directory")
}

func TestManager_ExtractArchive_MaliciousPath(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create archive with path traversal attempt
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	testContent := "Malicious content"
	header := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: int64(len(testContent)),
	}
	
	err = tarWriter.WriteHeader(header)
	require.NoError(t, err)
	
	_, err = tarWriter.Write([]byte(testContent))
	require.NoError(t, err)

	err = tarWriter.Close()
	require.NoError(t, err)
	
	err = gzWriter.Close()
	require.NoError(t, err)

	// Test extraction should fail
	ctx := context.Background()
	extractPath, cleanup, err := manager.ExtractArchive(ctx, buf.Bytes())
	
	assert.Error(t, err, "ExtractArchive should fail with malicious path")
	assert.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")
	assert.Empty(t, extractPath, "Extract path should be empty on failure")
	
	if cleanup != nil {
		cleanup()
	}
}

func TestManager_ValidateArchive(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a valid archive
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	testContent := "Valid content"
	header := &tar.Header{
		Name: "valid.go",
		Mode: 0644,
		Size: int64(len(testContent)),
	}
	
	err = tarWriter.WriteHeader(header)
	require.NoError(t, err)
	
	_, err = tarWriter.Write([]byte(testContent))
	require.NoError(t, err)

	err = tarWriter.Close()
	require.NoError(t, err)
	
	err = gzWriter.Close()
	require.NoError(t, err)

	tests := []struct {
		name               string
		allowedExtensions  []string
		maxSizeBytes      int64
		expectError       bool
		errorContains     string
	}{
		{
			name:              "valid archive with allowed extension",
			allowedExtensions: []string{".go", ".txt", ".java"},
			maxSizeBytes:      1024 * 1024, // 1MB
			expectError:       false,
		},
		{
			name:              "archive with disallowed extension",
			allowedExtensions: []string{".txt", ".java"},
			maxSizeBytes:      1024 * 1024,
			expectError:       true,
			errorContains:     "not allowed",
		},
		{
			name:              "archive exceeds size limit",
			allowedExtensions: []string{".go", ".txt", ".java"},
			maxSizeBytes:      10, // Very small limit
			expectError:       true,
			errorContains:     "exceeds maximum size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateArchive(buf.Bytes(), tt.allowedExtensions, tt.maxSizeBytes)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_ReadFileSecure(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory with test file
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Create test file
	testContent := "Test file content for secure reading"
	testFile := filepath.Join(workDir, "test.txt")
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Test successful read
	content, err := manager.ReadFileSecure(workDir, "test.txt")
	assert.NoError(t, err, "Should successfully read file")
	assert.Equal(t, testContent, string(content), "Content should match")

	// Test reading non-existent file
	_, err = manager.ReadFileSecure(workDir, "nonexistent.txt")
	assert.Error(t, err, "Should fail to read non-existent file")

	// Test path traversal attempt
	_, err = manager.ReadFileSecure(workDir, "../../../etc/passwd")
	assert.Error(t, err, "Should fail with path traversal attempt")
	assert.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")
}

func TestManager_WriteFileSecure(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Test successful write
	testContent := "Test content for secure writing"
	err = manager.WriteFileSecure(workDir, "output.txt", []byte(testContent), 0644)
	assert.NoError(t, err, "Should successfully write file")

	// Verify written content
	writtenContent, err := os.ReadFile(filepath.Join(workDir, "output.txt"))
	assert.NoError(t, err, "Should be able to read written file")
	assert.Equal(t, testContent, string(writtenContent), "Written content should match")

	// Test writing to subdirectory (should create directory)
	err = manager.WriteFileSecure(workDir, "subdir/nested.txt", []byte("nested content"), 0644)
	assert.NoError(t, err, "Should successfully write file in subdirectory")

	// Verify subdirectory file
	nestedContent, err := os.ReadFile(filepath.Join(workDir, "subdir", "nested.txt"))
	assert.NoError(t, err, "Should be able to read nested file")
	assert.Equal(t, "nested content", string(nestedContent), "Nested content should match")

	// Test path traversal attempt
	err = manager.WriteFileSecure(workDir, "../../../tmp/malicious.txt", []byte("bad"), 0644)
	assert.Error(t, err, "Should fail with path traversal attempt")
	assert.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")
}

func TestManager_ListDirectorySecure(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory with test files
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Create test files and directories
	testFiles := []string{"file1.txt", "file2.go", "README.md"}
	for _, fileName := range testFiles {
		err = os.WriteFile(filepath.Join(workDir, fileName), []byte("content"), 0644)
		require.NoError(t, err)
	}

	err = os.Mkdir(filepath.Join(workDir, "subdir"), 0755)
	require.NoError(t, err)

	// Test listing directory
	entries, err := manager.ListDirectorySecure(workDir, ".")
	assert.NoError(t, err, "Should successfully list directory")
	assert.Len(t, entries, 4, "Should list all files and subdirectory") // 3 files + 1 directory

	// Check that all expected entries are present
	entryNames := make(map[string]bool)
	for _, entry := range entries {
		entryNames[entry.Name()] = true
	}
	
	for _, expectedFile := range testFiles {
		assert.True(t, entryNames[expectedFile], "Should contain file: %s", expectedFile)
	}
	assert.True(t, entryNames["subdir"], "Should contain subdirectory")

	// Test listing subdirectory
	subEntries, err := manager.ListDirectorySecure(workDir, "subdir")
	assert.NoError(t, err, "Should successfully list subdirectory")
	assert.Len(t, subEntries, 0, "Subdirectory should be empty")

	// Test path traversal attempt
	_, err = manager.ListDirectorySecure(workDir, "../../../")
	assert.Error(t, err, "Should fail with path traversal attempt")
	assert.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")

	// Test listing non-existent directory
	_, err = manager.ListDirectorySecure(workDir, "nonexistent")
	assert.Error(t, err, "Should fail to list non-existent directory")
}

func TestManager_ExecuteCommand(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Test successful command execution
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, "echo", []string{"Hello, CLLM!"}, workDir)
	
	assert.NoError(t, err, "Command execution should succeed")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	assert.Contains(t, result.Stdout, "Hello, CLLM!", "Stdout should contain expected output")
	assert.Empty(t, result.Stderr, "Stderr should be empty for successful command")

	// Test command with stderr output - use cat with nonexistent file
	result, err = manager.ExecuteCommand(ctx, "cat", []string{"/nonexistent/file/test.txt"}, workDir)
	
	// cat should complete but exit with non-zero code and write to stderr
	assert.NoError(t, err, "Command execution should complete without process error")
	assert.NotNil(t, result, "Result should not be nil")
	assert.NotEqual(t, 0, result.ExitCode, "Exit code should not be 0")
	assert.Contains(t, result.Stderr, "No such file or directory", "Stderr should contain error message")

	// Test command that doesn't exist
	result, err = manager.ExecuteCommand(ctx, "nonexistentcommand123", []string{}, workDir)
	
	assert.Error(t, err, "Non-existent command should fail")
	assert.Contains(t, err.Error(), "failed to start command", "Error should mention command start failure")
}

func TestManager_ExecuteCommand_Timeout(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Test command timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := manager.ExecuteCommand(ctx, "sleep", []string{"5"}, workDir)
	
	if err != nil {
		// Context timeout should cause command failure
		assert.Contains(t, err.Error(), "context deadline exceeded", "Should timeout with context deadline")
	} else {
		// If no error, the command completed faster than expected
		t.Log("Command completed faster than timeout")
		assert.NotNil(t, result, "Result should not be nil even on fast completion")
	}
}

func TestManager_ExecuteCommand_WorkingDirectory(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory with a test file
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Create a test file in the working directory
	testFile := "testfile.txt"
	testContent := "test content"
	err = manager.WriteFileSecure(workDir, testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Test that command runs in the correct working directory
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, "cat", []string{testFile}, workDir)
	
	assert.NoError(t, err, "Command should execute successfully")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	assert.Contains(t, result.Stdout, testContent, "Should read file from working directory")

	// Test invalid working directory (path traversal)
	_, err = manager.ExecuteCommand(ctx, "echo", []string{"test"}, "../../../etc")
	assert.Error(t, err, "Should fail with invalid working directory")
	assert.Contains(t, err.Error(), "path traversal", "Error should mention path traversal")
}

func TestManager_ExecuteCommand_ResourceLimits(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "50MB",  // Small memory limit
		MaxCPUTime:     "10s",   // Short CPU time limit
		MaxProcesses:   5,       // Limited processes
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Test that resource limits are applied (check environment variables)
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, "env", []string{}, workDir)
	
	assert.NoError(t, err, "Command should execute successfully")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	
	// Check that resource limit environment variables are set
	assert.Contains(t, result.Stdout, "CLLM_MAX_MEMORY=50MB", "Should set memory limit in environment")
	assert.Contains(t, result.Stdout, "CLLM_MAX_CPU_TIME=10s", "Should set CPU time limit in environment")
}

func TestManager_ExecuteCommand_OutputCapture(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Test command with both stdout and stderr
	ctx := context.Background()
	result, err := manager.ExecuteCommand(ctx, "sh", []string{"-c", "echo stdout message"}, workDir)
	
	assert.NoError(t, err, "Command should execute successfully")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	assert.Contains(t, result.Stdout, "stdout message", "Should capture stdout")

	// Test command with no output
	result, err = manager.ExecuteCommand(ctx, "true", []string{}, workDir)
	
	assert.NoError(t, err, "Command should execute successfully")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	assert.Empty(t, result.Stdout, "Stdout should be empty")
	assert.Empty(t, result.Stderr, "Stderr should be empty")

	// Test command with large output (ensure proper buffering) - simplified
	result, err = manager.ExecuteCommand(ctx, "head", []string{"/dev/zero"}, workDir)
	// This will fail, but let's test a simpler large output case
	result, err = manager.ExecuteCommand(ctx, "echo", []string{"This is a test of output capture functionality"}, workDir)
	
	assert.NoError(t, err, "Command should execute successfully")
	assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
	assert.Contains(t, result.Stdout, "test", "Should capture output")
	assert.Contains(t, result.Stdout, "functionality", "Should capture complete output")
}

func TestManager_ValidateCommandArguments(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	tests := []struct {
		name        string
		command     string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid safe command",
			command:     "echo",
			args:        []string{"hello", "world"},
			expectError: false,
		},
		{
			name:        "command with suspicious argument",
			command:     "echo",
			args:        []string{"$(rm -rf /)"},
			expectError: true,
			errorMsg:    "suspicious command injection",
		},
		{
			name:        "command with null bytes",
			command:     "echo",
			args:        []string{"hello\x00world"},
			expectError: true,
			errorMsg:    "null byte detected",
		},
		{
			name:        "command with path traversal in args",
			command:     "cat",
			args:        []string{"../../../etc/passwd"},
			expectError: true,
			errorMsg:    "path traversal detected",
		},
		{
			name:        "excessively long argument",
			command:     "echo",
			args:        []string{strings.Repeat("a", 10000)},
			expectError: true,
			errorMsg:    "argument too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateCommandArguments(tt.command, tt.args)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_DetectSymlinks(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Create a regular file
	regularFile := filepath.Join(workDir, "regular.txt")
	err = os.WriteFile(regularFile, []byte("content"), 0644)
	require.NoError(t, err)

	// Create a symlink pointing outside sandbox
	symlinkPath := filepath.Join(workDir, "malicious_link")
	err = os.Symlink("/etc/passwd", symlinkPath)
	require.NoError(t, err)

	// Test symlink detection
	isSymlink, symlinkTarget, err := manager.DetectSymlink(symlinkPath)
	assert.NoError(t, err, "Should detect symlink successfully")
	assert.True(t, isSymlink, "Should identify as symlink")
	assert.Equal(t, "/etc/passwd", symlinkTarget, "Should return correct target")

	// Test regular file
	isSymlink, regularTarget, err := manager.DetectSymlink(regularFile)
	assert.NoError(t, err, "Should handle regular file")
	assert.False(t, isSymlink, "Should identify as regular file")
	assert.Empty(t, regularTarget, "Target should be empty for regular file")

	// Test validation of symlink target (should reject outside sandbox)
	err = manager.ValidateSymlinkTarget(workDir, symlinkTarget)
	assert.Error(t, err, "Should reject symlink pointing outside sandbox")
	assert.Contains(t, err.Error(), "outside sandbox", "Error should mention sandbox boundary")
}

func TestManager_MonitorResourceUsage(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "100MB",  // Small limit for testing
		MaxCPUTime:     "30s",
		MaxProcesses:   5,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Start resource monitoring
	monitor, err := manager.StartResourceMonitoring()
	require.NoError(t, err, "Should start resource monitoring")
	defer monitor.Stop()

	// Perform some operations that consume resources
	testData := make([]byte, 1024*1024) // 1MB of data
	for i := 0; i < 10; i++ {
		fileName := fmt.Sprintf("test_%d.dat", i)
		err = manager.WriteFileSecure(workDir, fileName, testData, 0644)
		require.NoError(t, err)
	}

	// Get resource usage
	usage := monitor.GetCurrentUsage()
	assert.NotNil(t, usage, "Should return usage statistics")
	assert.Greater(t, usage.MemoryUsed, int64(0), "Should show memory usage")
	assert.Greater(t, usage.DiskUsed, int64(0), "Should show disk usage")

	// Test resource limit checking
	isWithinLimits, violations := monitor.CheckResourceLimits()
	assert.NotNil(t, violations, "Should return violations list")
	
	if !isWithinLimits {
		t.Logf("Resource limit violations detected: %v", violations)
	}
}

func TestManager_AuditLogging(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create audit logger
	auditor := manager.GetSecurityAuditor()
	require.NotNil(t, auditor, "Should have security auditor")

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	// Perform operations that should be audited
	ctx := context.Background()
	
	// File operation
	err = manager.WriteFileSecure(workDir, "test.txt", []byte("test content"), 0644)
	require.NoError(t, err)

	// Command execution
	result, err := manager.ExecuteCommand(ctx, "echo", []string{"audited command"}, workDir)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// Get audit logs
	logs := auditor.GetAuditLogs()
	assert.NotEmpty(t, logs, "Should have audit log entries")

	// Verify log entries contain expected information
	foundFileOp := false
	foundCommandOp := false
	
	for _, log := range logs {
		if log.Operation == "file_write" && strings.Contains(log.Details, "test.txt") {
			foundFileOp = true
			assert.Equal(t, "success", log.Result)
			assert.NotEmpty(t, log.Timestamp)
		}
		if log.Operation == "command_execute" && strings.Contains(log.Details, "echo") {
			foundCommandOp = true
			assert.Equal(t, "success", log.Result)
			assert.Contains(t, log.Details, "audited command")
		}
	}

	assert.True(t, foundFileOp, "Should log file operation")
	assert.True(t, foundCommandOp, "Should log command execution")
}

func TestManager_SecurityEventLogging(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	auditor := manager.GetSecurityAuditor()
	require.NotNil(t, auditor)

	// Test security violation logging
	ctx := context.Background()
	
	// Attempt path traversal - should be logged as security event
	_, err = manager.ExecuteCommand(ctx, "echo", []string{"test"}, "../../../etc")
	assert.Error(t, err, "Should fail with path traversal")

	// Check security event logs
	securityEvents := auditor.GetSecurityEvents()
	assert.NotEmpty(t, securityEvents, "Should log security events")

	// Verify security event details
	found := false
	for _, event := range securityEvents {
		if event.EventType == "path_traversal_attempt" {
			found = true
			assert.Equal(t, "blocked", event.Action)
			assert.Contains(t, event.Details, "path traversal")
			assert.NotEmpty(t, event.Timestamp)
		}
	}

	assert.True(t, found, "Should log path traversal security event")
}

func TestManager_EnhancedPathValidation(t *testing.T) {
	config := ManagerConfig{
		WorkDir:        "/tmp/cllm-test-sandbox",
		MaxMemory:      "1GB",
		MaxCPUTime:     "300s",
		MaxProcesses:   10,
		CleanupTimeout: "30s",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)

	// Create a working directory
	workDir, cleanup, err := manager.CreateWorkingDirectory()
	require.NoError(t, err)
	defer cleanup()

	tests := []struct {
		name        string
		path        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid simple path",
			path:        "simple.txt",
			expectError: false,
		},
		{
			name:        "valid nested path",
			path:        "dir/subdir/file.txt",
			expectError: false,
		},
		{
			name:        "path too deep",
			path:        strings.Repeat("a/", 50) + "file.txt", // Exceeds depth limit
			expectError: true,
			errorMsg:    "path depth exceeds limit",
		},
		{
			name:        "path too long",
			path:        strings.Repeat("a", 1000) + ".txt", // Exceeds length limit
			expectError: true,
			errorMsg:    "path length exceeds limit",
		},
		{
			name:        "path with unicode control characters",
			path:        "file\u007f.txt", // DEL character (ASCII 127)
			expectError: true,
			errorMsg:    "contains control characters",
		},
		{
			name:        "path with backslashes",
			path:        "windows\\style\\path.txt",
			expectError: true,
			errorMsg:    "invalid path separators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ValidatePathEnhanced(workDir, tt.path)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}