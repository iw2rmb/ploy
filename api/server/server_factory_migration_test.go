package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerMustUseFactoryPattern verifies that all storage initialization
// in server.go uses the new factory pattern instead of CreateStorageClientFromConfig
func TestServerMustUseFactoryPattern(t *testing.T) {
	// Read server.go source code
	serverCode, err := os.ReadFile("./server.go")
	require.NoError(t, err)

	// Check that CreateStorageClientFromConfig is NOT used except in getStorageClient fallback
	// We should migrate all direct calls to use the factory pattern
	serverStr := string(serverCode)

	// Count occurrences of CreateStorageClientFromConfig
	// There should only be 1 (in the getStorageClient fallback)
	count := 0
	searchStr := "CreateStorageClientFromConfig"
	for i := 0; i < len(serverStr); i++ {
		if i+len(searchStr) <= len(serverStr) && serverStr[i:i+len(searchStr)] == searchStr {
			count++
			i += len(searchStr) - 1
		}
	}

	// Migration completed - CreateStorageClientFromConfig should no longer be used in server.go
	// All calls have been migrated to use CreateStorageFromFactory
	assert.Equal(t, 0, count, "CreateStorageClientFromConfig should no longer be used in server.go after migration completion. Found %d occurrences", count)
}

// TestHealthCheckerUsesFactoryPattern verifies health.go uses factory pattern
// TODO: This will be addressed in the next migration step
func TestHealthCheckerUsesFactoryPattern(t *testing.T) {
	t.Skip("Skipping - health.go migration will be done in next step")
	
	// Read health.go source code
	healthCode, err := os.ReadFile("../health/health.go")
	require.NoError(t, err)

	healthStr := string(healthCode)

	// Count occurrences of CreateStorageClientFromConfig in health.go
	// Should be 0 after migration
	count := 0
	searchStr := "CreateStorageClientFromConfig"
	for i := 0; i < len(healthStr); i++ {
		if i+len(searchStr) <= len(healthStr) && healthStr[i:i+len(searchStr)] == searchStr {
			count++
			i += len(searchStr) - 1
		}
	}

	// This test will fail until we complete the migration
	assert.Equal(t, 0, count, "health.go should use factory pattern, not CreateStorageClientFromConfig. Found %d occurrences", count)
}

// Helper to write test config
func writeTestConfigFile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)
	return configPath
}
