package server

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// Ensures storage client resolution requires centralized config service (no legacy fallback)
func TestServer_GetStorageClient_RequiresConfigService(t *testing.T) {
    srv := &Server{
        dependencies: &ServiceDependencies{StorageConfigPath: "/tmp/missing.yaml"},
    }
    // No configService injected
    _, err := srv.getStorageClient()
    require.Error(t, err, "expected error when config service is not available")
}

