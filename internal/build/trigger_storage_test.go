package build

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/envstore"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTriggerBuildWithUnifiedStorage tests that TriggerBuild can work with unified storage
func TestTriggerBuildWithUnifiedStorage(t *testing.T) {
	// This test expects new functions that accept storage.Storage interface

	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := mocks.NewEnvStore()

	// Mock environment store response
	envVars := envstore.AppEnvVars{
		"TEST_VAR": "test_value",
	}
	mockEnvStore.On("GetAll", "test-app").Return(envVars, nil)

	// Create test fiber app
	app := fiber.New()
	app.Post("/builds/:app", func(c *fiber.Ctx) error {
		// This will use the new function that accepts unified storage
		return TriggerBuildWithStorage(c, mockStorage, mockEnvStore)
	})

	// Create test request with minimal tar content
	tarContent := []byte("test tar content")
	req := httptest.NewRequest("POST", "/builds/test-app?sha=test-sha&lane=E", bytes.NewReader(tarContent))
	req.Header.Set("Content-Type", "application/x-tar")

	// Execute request (will fail in RED phase as function doesn't exist)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check response (expect success after implementation)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestTriggerPlatformBuildWithUnifiedStorage tests platform build with unified storage
func TestTriggerPlatformBuildWithUnifiedStorage(t *testing.T) {
	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := mocks.NewEnvStore()

	// Mock environment store
	mockEnvStore.On("GetAll", "platform-service").Return(envstore.AppEnvVars{}, nil)

	// Create test fiber app
	app := fiber.New()
	app.Post("/platform-builds/:app", func(c *fiber.Ctx) error {
		// This will use the new function for platform builds
		return TriggerPlatformBuildWithStorage(c, mockStorage, mockEnvStore)
	})

	// Create test request
	tarContent := []byte("platform service tar")
	req := httptest.NewRequest("POST", "/platform-builds/platform-service?lane=E", bytes.NewReader(tarContent))

	// Execute request (will fail in RED phase)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Expect success after implementation
	assert.Equal(t, 200, resp.StatusCode)
}

// TestTriggerAppBuildWithUnifiedStorage tests app build with unified storage
func TestTriggerAppBuildWithUnifiedStorage(t *testing.T) {
	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := mocks.NewEnvStore()

	// Mock environment store
	mockEnvStore.On("GetAll", "user-app").Return(envstore.AppEnvVars{}, nil)

	// Create test fiber app
	app := fiber.New()
	app.Post("/app-builds/:app", func(c *fiber.Ctx) error {
		// This will use the new function for app builds
		return TriggerAppBuildWithStorage(c, mockStorage, mockEnvStore)
	})

	// Create test request
	tarContent := []byte("user app tar")
	req := httptest.NewRequest("POST", "/app-builds/user-app?lane=C", bytes.NewReader(tarContent))

	// Execute request (will fail in RED phase)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Expect success after implementation
	assert.Equal(t, 200, resp.StatusCode)
}

// TestBackwardCompatibilityBuildTriggers tests that legacy functions still work
func TestBackwardCompatibilityBuildTriggers(t *testing.T) {
	// Ensure backward compatibility is maintained

	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("GetAll", "legacy-app").Return(envstore.AppEnvVars{}, nil)

	// Create test fiber app
	app := fiber.New()
	app.Post("/builds/:app", func(c *fiber.Ctx) error {
		// Original function should still work with nil StorageClient
		return TriggerBuild(c, nil, mockEnvStore)
	})

	// Create test request
	tarContent := []byte("legacy tar")
	req := httptest.NewRequest("POST", "/builds/legacy-app", bytes.NewReader(tarContent))

	// Execute request
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should handle gracefully even with nil storage
	// (might return error but shouldn't crash)
	assert.NotEqual(t, 500, resp.StatusCode)
}

// TestBuildDependenciesWithUnifiedStorage tests that BuildDependencies works with unified storage
func TestBuildDependenciesWithUnifiedStorage(t *testing.T) {
	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := mocks.NewEnvStore()

	// Create dependencies with unified storage
	deps := &BuildDependencies{
		Storage:  mockStorage,
		EnvStore: mockEnvStore,
	}

	// Verify dependencies are correctly set
	assert.NotNil(t, deps.Storage)
	assert.NotNil(t, deps.EnvStore)
	assert.Nil(t, deps.StorageClient) // Legacy client should be nil

	// Create build context
	buildCtx := &BuildContext{
		APIContext: "apps",
		AppType:    config.UserApp,
	}

	assert.NotNil(t, buildCtx)
}

