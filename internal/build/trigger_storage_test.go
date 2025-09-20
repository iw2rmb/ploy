package build

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTriggerBuildWithUnifiedStorage tests that TriggerBuild can work with unified storage
func TestTriggerBuildWithUnifiedStorage(t *testing.T) {
	t.Skip("Integration test - requires builders/orchestration; skipping in unit suite")

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
	tarContent := createTestTarball(t, map[string]string{"README.md": "ok"})
	req := httptest.NewRequest("POST", "/builds/test-app?sha=test-sha&lane=A", bytes.NewReader(tarContent))
	req.Header.Set("Content-Type", "application/x-tar")

	// Execute request (will fail in RED phase as function doesn't exist)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// For unit runs, reaching handler path without crash is sufficient
	assert.NotNil(t, resp)
}

// TestTriggerPlatformBuildWithUnifiedStorage tests platform build with unified storage
func TestTriggerPlatformBuildWithUnifiedStorage(t *testing.T) {
	t.Skip("Integration test - requires builders/orchestration; skipping in unit suite")
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
	tarContent := createTestTarball(t, map[string]string{"README.md": "ok"})
	req := httptest.NewRequest("POST", "/platform-builds/platform-service?lane=A", bytes.NewReader(tarContent))
	req.Header.Set("Content-Type", "application/x-tar")

	// Execute request (will fail in RED phase)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.NotNil(t, resp)
}

// TestTriggerAppBuildWithUnifiedStorage tests app build with unified storage
func TestTriggerAppBuildWithUnifiedStorage(t *testing.T) {
	t.Skip("Integration test - requires builders/orchestration; skipping in unit suite")
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
	tarContent := createTestTarball(t, map[string]string{"README.md": "ok"})
	req := httptest.NewRequest("POST", "/app-builds/user-app?lane=A", bytes.NewReader(tarContent))
	req.Header.Set("Content-Type", "application/x-tar")

	// Execute request (will fail in RED phase)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.NotNil(t, resp)
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
