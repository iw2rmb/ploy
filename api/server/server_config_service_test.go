package server

import (
    "os"
    "path/filepath"
    "testing"

    cfgsvc "github.com/iw2rmb/ploy/internal/config"
    "github.com/stretchr/testify/require"
)

func TestResolveStorageFromConfigService_UsesServiceWhenProvided(t *testing.T) {
    t.Parallel()

    // Prepare a minimal config file with memory provider
    dir := t.TempDir()
    path := filepath.Join(dir, "config.yaml")
    content := []byte("storage:\n  provider: memory\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    // Build the config service from file
    svc, err := cfgsvc.New(cfgsvc.WithFile(path))
    require.NoError(t, err)

    // Exercise resolver (service is required)
    st, err := resolveStorageFromConfigService(svc)
    require.NoError(t, err)
    require.NotNil(t, st)

    // Basic smoke: Metrics() should be callable
    _ = st.Metrics()
}
