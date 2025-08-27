package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFramework_Setup(t *testing.T) {
	// Test that we can create a test server
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	server, err := framework.CreateTestServer("basic")
	require.NoError(t, err)
	assert.NotNil(t, server)

	// Test server should be running and accessible
	assert.True(t, framework.IsServerReady(server))
}

func TestIntegrationFramework_HTTPClient(t *testing.T) {
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	// Test authenticated HTTP client creation
	client, err := framework.CreateHTTPClient("test-client-id", "test-auth-key")
	require.NoError(t, err)
	assert.NotNil(t, client)

	// Test unauthenticated client
	client, err = framework.CreateHTTPClient("", "")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestIntegrationFramework_TestFixtures(t *testing.T) {
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	tests := []struct {
		name        string
		fixtureType string
		expectError bool
	}{
		{
			name:        "valid python archive",
			fixtureType: "python-valid",
			expectError: false,
		},
		{
			name:        "python archive with errors",
			fixtureType: "python-errors",
			expectError: false,
		},
		{
			name:        "invalid archive",
			fixtureType: "invalid-archive",
			expectError: false,
		},
		{
			name:        "large archive",
			fixtureType: "large-archive",
			expectError: false,
		},
		{
			name:        "non-existent fixture",
			fixtureType: "non-existent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, err := framework.GetTestFixture(tt.fixtureType)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, fixture)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, fixture)
				assert.NotEmpty(t, fixture.Data)
				assert.NotEmpty(t, fixture.ContentType)
			}
		})
	}
}

func TestIntegrationFramework_ServerLifecycle(t *testing.T) {
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	server, err := framework.CreateTestServer("basic")
	require.NoError(t, err)

	// Test server start
	err = framework.StartServer(server)
	require.NoError(t, err)

	// Wait for server to be ready
	ready, err := framework.WaitForServerReady(server, 30)
	require.NoError(t, err)
	assert.True(t, ready)

	// Test server stop
	err = framework.StopServer(server)
	require.NoError(t, err)
}

func TestIntegrationFramework_ConfigManager(t *testing.T) {
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	// Test creating test configurations
	configs := []struct {
		name           string
		configType     string
		expectError    bool
	}{
		{
			name:        "basic config",
			configType:  "basic",
			expectError: false,
		},
		{
			name:        "streaming enabled",
			configType:  "streaming",
			expectError: false,
		},
		{
			name:        "auth enabled",
			configType:  "auth",
			expectError: false,
		},
		{
			name:        "rate limiting",
			configType:  "rate-limiting",
			expectError: false,
		},
		{
			name:        "invalid config",
			configType:  "invalid",
			expectError: true,
		},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			configPath, cleanup, err := framework.CreateTestConfig(cfg.configType)
			
			if cfg.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.FileExists(t, configPath)
				defer cleanup()
			}
		})
	}
}

func TestIntegrationFramework_TestUtilities(t *testing.T) {
	framework := NewIntegrationFramework()
	defer framework.Cleanup()

	// Test archive creation utilities
	archive, err := framework.CreateTestArchive([]TestFile{
		{Path: "test.py", Content: "print('hello world')"},
		{Path: "requirements.txt", Content: "requests==2.28.0"},
	})
	require.NoError(t, err)
	assert.Greater(t, len(archive), 0)

	// Test malformed archive creation
	malformedArchive := framework.CreateMalformedArchive()
	assert.Greater(t, len(malformedArchive), 0)

	// Test large archive creation
	largeArchive, err := framework.CreateLargeArchive(10 * 1024 * 1024) // 10MB
	require.NoError(t, err)
	assert.Greater(t, len(largeArchive), 10*1024*1024)
}