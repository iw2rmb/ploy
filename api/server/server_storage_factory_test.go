package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/api/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerUsesStorageFactory(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-storage-config.yaml")

	// Write test configuration
	configYAML := `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
  replication: "000"
  timeout: 30
  client:
    enable_metrics: true
    retry_config:
      max_retries: 3
      initial_delay: "100ms"
      max_delay: "30s"
      multiplier: 2.0
`
	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Test that server uses CreateStorageFromFactory instead of CreateStorageClientFromConfig
	t.Run("CreateStorageFromFactory should be used", func(t *testing.T) {
		// This test will fail until we implement the change
		storage, err := config.CreateStorageFromFactory(configPath)
		require.NoError(t, err)
		require.NotNil(t, storage)

		// Verify it's using the new interface
		metrics := storage.Metrics()
		assert.NotNil(t, metrics)
	})
}

func TestServerGetStorageClientMigration(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-storage-config.yaml")

	// Write test configuration
	configYAML := `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
`
	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Create server
	server := &Server{
		dependencies: &ServiceDependencies{
			StorageConfigPath: configPath,
			// When StorageFactory is nil, it should fall back to CreateStorageClientFromConfig
			StorageFactory: nil,
		},
	}

	// This will use the old path (CreateStorageClientFromConfig)
	client, err := server.getStorageClient()
	require.NoError(t, err)
	require.NotNil(t, client)

	// Now test with factory
	server.dependencies.StorageFactory = config.NewOptimizedStorageClientFactory(configPath)

	// This should use the factory path
	clientWithFactory, err := server.getStorageClient()
	require.NoError(t, err)
	require.NotNil(t, clientWithFactory)
}
