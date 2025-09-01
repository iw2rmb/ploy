package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerUsesFactoryBasedStorage(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-storage-config.yaml")

	// Write test configuration with seaweedfs (current requirement)
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
	err := writeTestConfig(configPath, configYAML)
	require.NoError(t, err)

	// Test that server creates storage using factory pattern
	cfg := &ControllerConfig{
		StorageConfigPath: configPath,
		EnableCaching:     true,
	}

	// Initialize server dependencies
	deps, err := initializeDependencies(cfg)
	require.NoError(t, err)
	require.NotNil(t, deps)

	// Verify StorageFactory is initialized when caching is enabled
	assert.NotNil(t, deps.StorageFactory, "StorageFactory should be initialized when caching is enabled")
}

func TestServerGetStorageClientUsesFactory(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-storage-config.yaml")

	// Write test configuration with SeaweedFS provider (as current implementation expects)
	configYAML := `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
  client:
    enable_metrics: true
    retry_config:
      max_retries: 5
      initial_delay: "200ms"
`
	err := writeTestConfig(configPath, configYAML)
	require.NoError(t, err)

	// Create server with factory-based storage
	server := &Server{
		dependencies: &ServiceDependencies{
			StorageConfigPath: configPath,
			StorageFactory:    config.NewOptimizedStorageClientFactory(configPath),
		},
	}

	// Test getStorageClient method
	client, err := server.getStorageClient()
	// We expect this to work with the factory
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify it returns StorageClient (current implementation)
	assert.NotNil(t, client)
}

func TestServerStorageFactoryConfiguration(t *testing.T) {
	tests := []struct {
		name             string
		configYAML       string
		expectRetry      bool
		expectMonitoring bool
		expectCaching    bool
	}{
		{
			name: "with retry middleware",
			configYAML: `
storage:
  provider: memory
  collection: "test"
  client:
    retry_config:
      max_retries: 3
`,
			expectRetry: true,
		},
		{
			name: "with monitoring middleware",
			configYAML: `
storage:
  provider: memory
  collection: "test"
  client:
    enable_metrics: true
`,
			expectMonitoring: true,
		},
		{
			name: "with all middleware",
			configYAML: `
storage:
  provider: memory
  collection: "test"
  client:
    enable_metrics: true
    retry_config:
      max_retries: 3
`,
			expectRetry:      true,
			expectMonitoring: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := writeTestConfig(configPath, tt.configYAML)
			require.NoError(t, err)

			// Create storage using factory
			storageClient, err := config.CreateStorageFromFactory(configPath)
			require.NoError(t, err)
			require.NotNil(t, storageClient)

			// Verify middleware is applied through metrics
			metrics := storageClient.Metrics()
			if tt.expectMonitoring {
				assert.NotNil(t, metrics, "Monitoring middleware should provide metrics")
			}

			// Verify storage operations work
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = storageClient.Health(ctx)
			// Memory provider always returns nil for Health
			assert.NoError(t, err)
		})
	}
}

// Helper function to write test configuration
func writeTestConfig(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
