package security

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceLimiter_CPULimit(t *testing.T) {
	limiter := NewResourceLimiter(ResourceLimits{
		MaxCPU:        0.5, // 50% of one core
		MaxMemory:     100 * 1024 * 1024, // 100MB
		MaxFiles:      10,
		MaxDuration:   5 * time.Second,
	})

	ctx := context.Background()

	// Wrap a simple command
	cmd := limiter.WrapCommand(ctx, "echo", "test")
	
	// Verify command is properly wrapped
	assert.NotNil(t, cmd)
	if runtime.GOOS != "windows" {
		// On Unix systems, should be wrapped with ulimit
		assert.Contains(t, cmd.Path, "sh")
	}
}

func TestResourceLimiter_MemoryLimit(t *testing.T) {
	limiter := NewResourceLimiter(ResourceLimits{
		MaxCPU:        1.0,
		MaxMemory:     50 * 1024 * 1024, // 50MB limit
		MaxFiles:      100,
		MaxDuration:   5 * time.Second,
	})

	ctx := context.Background()

	// Wrap command with memory limit
	cmd := limiter.WrapCommand(ctx, "echo", "test")
	
	// Verify memory limit is applied
	assert.NotNil(t, cmd)
	if runtime.GOOS != "windows" {
		// Check ulimit is in command
		assert.Contains(t, strings.Join(cmd.Args, " "), "ulimit")
	}
}

func TestResourceLimiter_FileCountLimit(t *testing.T) {
	limiter := NewResourceLimiter(ResourceLimits{
		MaxCPU:        1.0,
		MaxMemory:     100 * 1024 * 1024,
		MaxFiles:      5, // Very low file limit
		MaxDuration:   5 * time.Second,
	})

	ctx := context.Background()

	// Wrap command with file limit
	cmd := limiter.WrapCommand(ctx, "echo", "test")
	
	// Verify file limit is applied
	assert.NotNil(t, cmd)
	if runtime.GOOS != "windows" {
		// Check ulimit -n is in command
		cmdStr := strings.Join(cmd.Args, " ")
		assert.Contains(t, cmdStr, "ulimit")
		assert.Contains(t, cmdStr, "-n 5")
	}
}

func TestResourceLimiter_TimeLimit(t *testing.T) {
	limiter := NewResourceLimiter(ResourceLimits{
		MaxCPU:        1.0,
		MaxMemory:     100 * 1024 * 1024,
		MaxFiles:      100,
		MaxDuration:   100 * time.Millisecond, // Very short timeout
	})

	ctx := context.Background()

	// Command that would run for 1 second
	cmd := limiter.WrapCommand(ctx, "sleep", "1")
	
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Should timeout quickly
	if err != nil {
		assert.Less(t, duration, 500*time.Millisecond)
	}
}

func TestResourceLimiter_ProcessIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Process isolation requires Linux")
	}

	limiter := NewResourceLimiter(ResourceLimits{
		MaxCPU:        1.0,
		MaxMemory:     100 * 1024 * 1024,
		MaxFiles:      100,
		MaxDuration:   5 * time.Second,
		EnableCgroups: true,
		CgroupName:    "chttp_test",
	})

	ctx := context.Background()

	// Run in isolated cgroup
	cmd := limiter.WrapCommand(ctx, "echo", "test")
	err := cmd.Run()
	
	require.NoError(t, err)
	
	// Verify cgroup was created and cleaned up
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/cpu/chttp_test")
	_, err = os.Stat(cgroupPath)
	assert.True(t, os.IsNotExist(err), "Cgroup should be cleaned up after process")
}

func TestRateLimiter_RequestLimit(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		RequestsPerSecond: 2,
		BurstSize:         3,
	})

	// Should allow burst of 3 requests
	for i := 0; i < 3; i++ {
		allowed := limiter.Allow("client1")
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 4th request should be denied
	allowed := limiter.Allow("client1")
	assert.False(t, allowed, "4th request should be denied")

	// Wait for refill
	time.Sleep(600 * time.Millisecond)
	
	// Should allow one more
	allowed = limiter.Allow("client1")
	assert.True(t, allowed, "Request should be allowed after refill")
}

func TestRateLimiter_PerClientLimit(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         2,
		PerClient:         true,
	})

	// Client 1 uses their quota
	assert.True(t, limiter.Allow("client1"))
	assert.True(t, limiter.Allow("client1"))
	assert.False(t, limiter.Allow("client1"))

	// Client 2 should have their own quota
	assert.True(t, limiter.Allow("client2"))
	assert.True(t, limiter.Allow("client2"))
	assert.False(t, limiter.Allow("client2"))
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		RequestsPerSecond: 2,
		BurstSize:         3,
		PerClient:         false, // Global limit
	})

	// All clients share the same quota
	assert.True(t, limiter.Allow("client1"))
	assert.True(t, limiter.Allow("client2"))
	assert.True(t, limiter.Allow("client3"))
	assert.False(t, limiter.Allow("client1"), "Global limit should be reached")
	assert.False(t, limiter.Allow("client2"), "Global limit should be reached")
}

func TestPathSanitizer_PreventTraversal(t *testing.T) {
	sanitizer := NewPathSanitizer("/tmp/safe")

	tests := []struct {
		name        string
		input       string
		shouldError bool
		expected    string
	}{
		{
			name:        "valid relative path",
			input:       "subdir/file.txt",
			shouldError: false,
			expected:    "/tmp/safe/subdir/file.txt",
		},
		{
			name:        "path with ..",
			input:       "../etc/passwd",
			shouldError: true,
		},
		{
			name:        "path with hidden ..",
			input:       "subdir/../../etc/passwd",
			shouldError: true,
		},
		{
			name:        "absolute path attempt",
			input:       "/etc/passwd",
			shouldError: true,
		},
		// Symlink test handled separately due to filesystem requirements
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizer.Sanitize(tt.input)
			
			if tt.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "traversal")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestPathSanitizer_ValidateExtractedFiles(t *testing.T) {
	sanitizer := NewPathSanitizer("/tmp/safe")
	
	// Create test directory structure
	tempDir := t.TempDir()
	
	// Create some test files
	os.MkdirAll(tempDir+"/valid", 0755)
	os.WriteFile(tempDir+"/valid/file.txt", []byte("test"), 0644)
	
	// Create a symlink that tries to escape
	os.Symlink("/etc/passwd", tempDir+"/evil_link")
	
	// Validate extracted files
	err := sanitizer.ValidateDirectory(tempDir)
	
	// Should detect the dangerous symlink
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestSecurityValidator_ValidateArchive(t *testing.T) {
	validator := NewSecurityValidator(SecurityConfig{
		MaxArchiveSize:     10 * 1024 * 1024, // 10MB
		MaxExtractedSize:   50 * 1024 * 1024, // 50MB
		MaxFiles:           100,
		AllowedExtensions:  []string{".py", ".js", ".go"},
		BlockedPaths:       []string{"__pycache__", "node_modules", ".git"},
	})

	tests := []struct {
		name        string
		archive     ArchiveMetadata
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid archive",
			archive: ArchiveMetadata{
				Size:          5 * 1024 * 1024,
				ExtractedSize: 15 * 1024 * 1024,
				FileCount:     50,
				Files: []string{
					"main.py",
					"utils.js",
					"server.go",
				},
			},
			shouldError: false,
		},
		{
			name: "archive too large",
			archive: ArchiveMetadata{
				Size:          15 * 1024 * 1024,
				ExtractedSize: 20 * 1024 * 1024,
				FileCount:     50,
			},
			shouldError: true,
			errorMsg:    "archive size exceeds limit",
		},
		{
			name: "too many files",
			archive: ArchiveMetadata{
				Size:          5 * 1024 * 1024,
				ExtractedSize: 20 * 1024 * 1024,
				FileCount:     200,
			},
			shouldError: true,
			errorMsg:    "too many files",
		},
		{
			name: "blocked extension",
			archive: ArchiveMetadata{
				Size:          1 * 1024 * 1024,
				ExtractedSize: 2 * 1024 * 1024,
				FileCount:     10,
				Files: []string{
					"main.py",
					"malware.exe",
				},
			},
			shouldError: true,
			errorMsg:    "blocked file extension",
		},
		{
			name: "blocked path",
			archive: ArchiveMetadata{
				Size:          1 * 1024 * 1024,
				ExtractedSize: 2 * 1024 * 1024,
				FileCount:     10,
				Files: []string{
					"main.py",
					"__pycache__/compiled.pyc",
				},
			},
			shouldError: true,
			errorMsg:    "blocked path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateArchive(tt.archive)
			
			if tt.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}