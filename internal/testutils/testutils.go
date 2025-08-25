// Package testutils provides utilities for testing Ploy components
// This package includes mocks, builders, fixtures, and integration test helpers
package testutils

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConfig holds configuration for test environments
type TestConfig struct {
	// Database configuration
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDatabase string

	// Redis configuration
	RedisHost     string
	RedisPort     int
	RedisPassword string

	// Service endpoints
	ConsulEndpoint    string
	NomadEndpoint     string
	SeaweedFSMaster   string
	SeaweedFSFiler    string
	ControllerEndpoint string

	// Test directories
	TempDir     string
	FixturesDir string
	TestDataDir string
}

// DefaultTestConfig returns a default configuration for local testing
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		// Local Docker services
		PostgresHost:     "localhost",
		PostgresPort:     5432,
		PostgresUser:     "ploy",
		PostgresPassword: "ploy-test",
		PostgresDatabase: "ploy_test",

		RedisHost: "localhost",
		RedisPort: 6379,

		ConsulEndpoint:     "http://localhost:8500",
		NomadEndpoint:      "http://localhost:4646",
		SeaweedFSMaster:    "http://localhost:9333",
		SeaweedFSFiler:     "http://localhost:8888",
		ControllerEndpoint: "http://localhost:8081/v1",

		TempDir:     filepath.Join(os.TempDir(), "ploy-test"),
		FixturesDir: "internal/testutils/fixtures",
		TestDataDir: "test-data",
	}
}

// SetupTestEnvironment prepares the test environment
func SetupTestEnvironment(t *testing.T) *TestConfig {
	t.Helper()

	config := DefaultTestConfig()

	// Create temp directories
	err := os.MkdirAll(config.TempDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(config.TestDataDir, 0755)
	require.NoError(t, err)

	// Cleanup function
	t.Cleanup(func() {
		os.RemoveAll(config.TempDir)
	})

	return config
}

// TeardownTestEnvironment cleans up test resources
func TeardownTestEnvironment(config *TestConfig) {
	if config.TempDir != "" {
		os.RemoveAll(config.TempDir)
	}
}

// WaitForService waits for a service to be available
func WaitForService(endpoint string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for service at %s", endpoint)
		default:
			// Try to connect
			if isServiceReady(endpoint) {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// isServiceReady checks if a service is ready by attempting to connect
func isServiceReady(endpoint string) bool {
	// For HTTP endpoints, we could do an HTTP check
	// For now, just check if we can establish a TCP connection
	// This is a simplified check - in real implementation we'd parse the endpoint
	// and make appropriate HTTP requests
	
	// Extract host:port from common endpoint formats
	// This is a simplified parser for demonstration
	return true // Placeholder - would implement proper service health check
}

// GetFreePort returns a free port for testing
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

// CreateTempDir creates a temporary directory for testing
func CreateTempDir(t *testing.T, prefix string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", prefix)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return dir
}

// CreateTempFile creates a temporary file for testing
func CreateTempFile(t *testing.T, dir, pattern, content string) string {
	t.Helper()

	if dir == "" {
		dir = t.TempDir()
	}

	file, err := os.CreateTemp(dir, pattern)
	require.NoError(t, err)

	if content != "" {
		_, err = file.WriteString(content)
		require.NoError(t, err)
	}

	err = file.Close()
	require.NoError(t, err)

	return file.Name()
}

// Note: AssertEventually is defined in assertions.go

// SkipIfNoDocker skips the test if Docker is not available
func SkipIfNoDocker(t *testing.T) {
	t.Helper()

	// Simple check for Docker availability
	_, err := os.Stat("/var/run/docker.sock")
	if os.IsNotExist(err) {
		t.Skip("Docker is not available, skipping integration test")
	}
}

// SkipIfNoServices skips the test if local services are not available
func SkipIfNoServices(t *testing.T) {
	t.Helper()

	config := DefaultTestConfig()
	
	// Check if key services are available with short timeout
	services := map[string]string{
		"consul":    config.ConsulEndpoint,
		"nomad":     config.NomadEndpoint,
		"seaweedfs": config.SeaweedFSMaster,
	}

	for name, endpoint := range services {
		err := WaitForService(endpoint, 1*time.Second)
		if err != nil {
			t.Skipf("Service %s is not available at %s, skipping integration test", name, endpoint)
		}
	}
}