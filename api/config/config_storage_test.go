package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateStorageFromFactory(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	tests := []struct {
		name       string
		configYAML string
		wantError  bool
		errorMsg   string
		validate   func(t *testing.T, storage storage.Storage)
	}{
		{
			name: "valid seaweedfs config",
			configYAML: `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-artifacts"
  replication: "000"
  timeout: 30
  client:
    enable_metrics: true
    enable_health_check: false
    retry_config:
      max_retries: 3
      initial_delay: "100ms"
      max_delay: "30s"
      multiplier: 2.0
`,
			wantError: false,
			validate: func(t *testing.T, s storage.Storage) {
				assert.NotNil(t, s)
				// Should be able to call Health without error
				err := s.Health(nil)
				// May return error if not connected, but should not panic
				_ = err
			},
		},
		{
			name: "config with retry enabled",
			configYAML: `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
  client:
    retry_config:
      max_retries: 5
      initial_delay: "200ms"
      max_delay: "1m"
      multiplier: 3.0
`,
			wantError: false,
			validate: func(t *testing.T, s storage.Storage) {
				assert.NotNil(t, s)
				// The storage should have retry middleware applied
				metrics := s.Metrics()
				assert.NotNil(t, metrics)
			},
		},
		{
			name: "config with monitoring enabled",
			configYAML: `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
  client:
    enable_metrics: true
`,
			wantError: false,
			validate: func(t *testing.T, s storage.Storage) {
				assert.NotNil(t, s)
				// Should have metrics enabled
				metrics := s.Metrics()
				assert.NotNil(t, metrics)
			},
		},
		{
			name: "missing required fields",
			configYAML: `
storage:
  provider: seaweedfs
  # Missing master
  filer: "localhost:8888"
`,
			wantError: true,
			errorMsg:  "seaweedfs master address is required",
		},
		{
			name: "invalid provider",
			configYAML: `
storage:
  provider: invalid
  master: "localhost:9333"
`,
			wantError: true,
			errorMsg:  "unknown provider: invalid",
		},
		{
			name: "seaweedfs provider explicit",
			configYAML: `
storage:
  provider: "seaweedfs"
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-collection"
`,
			wantError: false,
			validate: func(t *testing.T, s storage.Storage) {
				assert.NotNil(t, s)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write config file
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			require.NoError(t, err)

			// Call the new factory-based function
			storage, err := CreateStorageFromFactory(configPath)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, storage)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, storage)
				if tt.validate != nil {
					tt.validate(t, storage)
				}
			}
		})
	}
}

func TestCreateStorageFromFactory_FileNotFound(t *testing.T) {
	storage, err := CreateStorageFromFactory("/non/existent/path.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load storage config")
	assert.Nil(t, storage)
}

func TestCreateStorageFromFactory_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644)
	require.NoError(t, err)

	storage, err := CreateStorageFromFactory(configPath)
	assert.Error(t, err)
	assert.Nil(t, storage)
}

// TestCreateStorageClientFromConfig_Compatibility tests that the old function
// still works but is marked as deprecated
func TestCreateStorageClientFromConfig_Compatibility(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configYAML := `
storage:
  provider: seaweedfs
  master: "localhost:9333"
  filer: "localhost:8888"
  collection: "test-artifacts"
  client:
    enable_metrics: true
    retry_config:
      max_retries: 3
`

	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// The old function should still work for backward compatibility
	client, err := CreateStorageClientFromConfig(configPath)

	// This will work with the existing implementation
	assert.NoError(t, err)
	assert.NotNil(t, client)
}
