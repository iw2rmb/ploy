package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/iw2rmb/ploy/internal/config"
	"github.com/stretchr/testify/require"
)

// Ensures /storage/health works via config Service even when file path is invalid
func TestStorageHealth_UsesConfigServiceWhenProvided(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("storage:\n  provider: memory\n"), 0o644))

	svc, err := cfg.New(cfg.WithFile(path))
	require.NoError(t, err)

	server := createMockServer()
	server.dependencies.StorageConfigPath = filepath.Join(dir, "missing.yaml")
	server.configService = svc

	server.app.Get("/storage/health", server.handleStorageHealth)

	req := httptest.NewRequest("GET", "/storage/health", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, 200, resp.StatusCode)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "healthy", body["status"])
}

// Ensures /storage/metrics works via config Service even when file path is invalid
func TestStorageMetrics_UsesConfigServiceWhenProvided(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("storage:\n  provider: memory\n"), 0o644))

	svc, err := cfg.New(cfg.WithFile(path))
	require.NoError(t, err)

	server := createMockServer()
	server.dependencies.StorageConfigPath = filepath.Join(dir, "missing.yaml")
	server.configService = svc

	server.app.Get("/storage/metrics", server.handleStorageMetrics)

	req := httptest.NewRequest("GET", "/storage/metrics", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, 200, resp.StatusCode)
	// Metrics JSON structure may evolve; presence check is sufficient here
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
}
