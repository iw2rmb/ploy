package server

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    cfgsvc "github.com/iw2rmb/ploy/internal/config"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestServerResolvesStorageViaConfigService(t *testing.T) {
    // Create a temporary config file using internal schema
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "test-storage-config.yaml")
    configYAML := `
storage:
  provider: memory
  endpoint: ""
  bucket: "artifacts"
`
    err := writeTestConfig(configPath, configYAML)
    require.NoError(t, err)

    // Create config service
    svc, err := cfgsvc.New(cfgsvc.WithFile(configPath), cfgsvc.WithValidation(cfgsvc.NewStructValidator()))
    require.NoError(t, err)

    // Minimal server with config service
    srv := &Server{configService: svc, dependencies: &ServiceDependencies{}}
    st, err := srv.getStorageClient()
    require.NoError(t, err)
    assert.NotNil(t, st)
}

func TestServerGetStorageClientUsesConfigService(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-storage-config.yaml")

    // Write test configuration using internal schema
    configYAML := `
storage:
  provider: memory
  bucket: "test"
`
	err := writeTestConfig(configPath, configYAML)
	require.NoError(t, err)

    // Create server with config service
    svc, err := cfgsvc.New(cfgsvc.WithFile(configPath), cfgsvc.WithValidation(cfgsvc.NewStructValidator()))
    require.NoError(t, err)
    server := &Server{configService: svc, dependencies: &ServiceDependencies{}}
    client, err := server.getStorageClient()
    require.NoError(t, err)
    require.NotNil(t, client)

	// Verify it returns StorageClient (current implementation)
	assert.NotNil(t, client)
}

func TestServerStorageConfiguration_MiddlewareMapping(t *testing.T) {
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
  bucket: "test"
  retry:
    enabled: true
    max_attempts: 3
    initial_delay: "100ms"
    max_delay: "2s"
    backoff_multiplier: 2.0
`,
            expectRetry: true,
        },
        {
            name: "with monitoring middleware",
            configYAML: `
storage:
  provider: memory
  bucket: "test"
  cache:
    enabled: true
    max_size: 10
    ttl: "1m"
`,
            expectMonitoring: true,
        },
        {
            name: "with all middleware",
            configYAML: `
storage:
  provider: memory
  bucket: "test"
  retry:
    enabled: true
    max_attempts: 2
  cache:
    enabled: true
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

            // Create storage using internal config service
            svc, err := cfgsvc.New(cfgsvc.WithFile(configPath))
            require.NoError(t, err)
            require.NotNil(t, svc)
            storageClient, err := svc.Get().CreateStorageClient()
            require.NoError(t, err)

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
