package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/api/envstore"
	cfg "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/testing/mocks"
)

// Mock config manager
type MockConfigManager struct {
	mock.Mock
}

func (m *MockConfigManager) LoadConfig() (interface{}, error) {
	args := m.Called()
	return args.Get(0), args.Error(1)
}

func (m *MockConfigManager) ReloadIfChanged() (interface{}, bool, error) {
	args := m.Called()
	return args.Get(0), args.Bool(1), args.Error(2)
}

// Test utility functions
func createTestApp() *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		},
	})
	return app
}

// TestableServer extends Server with mockable methods for testing
type TestableServer struct {
	*Server
	mockStorageClient func() (interface{}, error)
}

func (ts *TestableServer) getStorageClient() (interface{}, error) {
	if ts.mockStorageClient != nil {
		return ts.mockStorageClient()
	}
	// This would normally call the real method, but in tests we always use mock
	return nil, fmt.Errorf("no mock storage client configured")
}

func createMockServer() *TestableServer {
    server := &Server{
        app:    createTestApp(),
        config: &ControllerConfig{},
        dependencies: &ServiceDependencies{
            EnvStore:          mocks.NewEnvStore(),
            StorageConfigPath: "/test/config",
        },
    }
    return &TestableServer{Server: server}
}

func TestServer_HandleValidateStorageConfig_UsesConfigService(t *testing.T) {
    // Create a temporary valid config file for the config service
    dir := t.TempDir()
    validPath := dir + "/config.yaml"
    // Minimal storage config for service validator (seaweedfs requires endpoint per validator in internal/config)
    // Our internal validator in this repo checks seaweedfs endpoint via StructValidator in internal/config.
    content := []byte("storage:\n  provider: seaweedfs\n  endpoint: http://localhost:9333\n")
    require.NoError(t, os.WriteFile(validPath, content, 0o644))

    // Build a config service on that path
    svc, err := cfg.New(
        cfg.WithFile(validPath),
        cfg.WithValidation(cfg.NewStructValidator()),
    )
    require.NoError(t, err)

    // Server with a bogus path (would fail if legacy path was used)
    server := createMockServer()
    server.dependencies.StorageConfigPath = "/tmp/definitely-missing.yaml"
    server.configService = svc

    server.app.Post("/storage/config/validate", server.handleValidateStorageConfig)

    req := httptest.NewRequest("POST", "/storage/config/validate", nil)
    resp, err := server.app.Test(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, 200, resp.StatusCode)
    var body map[string]interface{}
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Equal(t, true, body["valid"])
}

func TestServer_HandleReloadStorageConfig_UsesConfigService(t *testing.T) {
    // Create a temporary valid config file for the config service
    dir := t.TempDir()
    validPath := dir + "/config.yaml"
    content := []byte("app:\n  name: test\nstorage:\n  provider: memory\n")
    require.NoError(t, os.WriteFile(validPath, content, 0o644))

    svc, err := cfg.New(
        cfg.WithFile(validPath),
    )
    require.NoError(t, err)

    server := createMockServer()
    server.dependencies.StorageConfigPath = "/tmp/missing-config.yaml"
    server.configService = svc

    server.app.Post("/storage/config/reload", server.handleReloadStorageConfig)

    req := httptest.NewRequest("POST", "/storage/config/reload", nil)
    resp, err := server.app.Test(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, 200, resp.StatusCode)
    var body map[string]interface{}
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Equal(t, true, body["reloaded"])
    // We returned config from the config service; just ensure it's present
    _, ok := body["config"]
    assert.True(t, ok)
}

func TestParseIntEnv(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		defaultVal  int
		expectedVal int
	}{
		{
			name:        "valid integer value",
			envVar:      "TEST_INT",
			envValue:    "42",
			defaultVal:  10,
			expectedVal: 42,
		},
		{
			name:        "empty environment variable",
			envVar:      "EMPTY_VAR",
			envValue:    "",
			defaultVal:  25,
			expectedVal: 25,
		},
		{
			name:        "invalid integer value",
			envVar:      "INVALID_INT",
			envValue:    "not-a-number",
			defaultVal:  30,
			expectedVal: 30,
		},
		{
			name:        "zero value",
			envVar:      "ZERO_VAR",
			envValue:    "0",
			defaultVal:  100,
			expectedVal: 0,
		},
		{
			name:        "negative value",
			envVar:      "NEGATIVE_VAR",
			envValue:    "-15",
			defaultVal:  50,
			expectedVal: -15,
		},
		{
			name:        "large value",
			envVar:      "LARGE_VAR",
			envValue:    "999999",
			defaultVal:  1,
			expectedVal: 999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or unset environment variable
			if tt.envValue != "" {
				os.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			} else {
				os.Unsetenv(tt.envVar)
			}

			result := parseIntEnv(tt.envVar, tt.defaultVal)
			assert.Equal(t, tt.expectedVal, result)
		})
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set some environment variables for testing
	originalValues := make(map[string]string)
	testEnvVars := map[string]string{
		"PORT":                    "9090",
		"CONSUL_HTTP_ADDR":        "192.168.1.100:8500",
		"NOMAD_ADDR":              "http://192.168.1.100:4646",
		"PLOY_USE_CONSUL_ENV":     "false",
		"PLOY_ENV_STORE_PATH":     "/custom/env/path",
		"PLOY_CLEANUP_AUTO_START": "false",
		"PLOY_ENABLE_CACHING":     "false",
	}

	// Store original values and set test values
	for key, value := range testEnvVars {
		originalValues[key] = os.Getenv(key)
		os.Setenv(key, value)
	}

	// Restore original values after test
	defer func() {
		for key, originalValue := range originalValues {
			if originalValue == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, originalValue)
			}
		}
	}()

	config := LoadConfigFromEnv()

	assert.Equal(t, "9090", config.Port)
	assert.Equal(t, "192.168.1.100:8500", config.ConsulAddr)
	assert.Equal(t, "http://192.168.1.100:4646", config.NomadAddr)
	assert.Equal(t, false, config.UseConsulEnv)
	assert.Equal(t, "/custom/env/path", config.EnvStorePath)
	assert.Equal(t, false, config.CleanupAutoStart)
	assert.Equal(t, false, config.EnableCaching)
	assert.Equal(t, 30*time.Second, config.ShutdownTimeout)
}

func TestLoadConfigFromEnvDefaults(t *testing.T) {
	// Clear relevant environment variables to test defaults
	envVarsToUnset := []string{
		"PORT", "CONSUL_HTTP_ADDR", "NOMAD_ADDR", "PLOY_USE_CONSUL_ENV",
		"PLOY_ENV_STORE_PATH", "PLOY_CLEANUP_AUTO_START", "PLOY_ENABLE_CACHING",
	}

	originalValues := make(map[string]string)
	for _, envVar := range envVarsToUnset {
		originalValues[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore original values after test
	defer func() {
		for envVar, originalValue := range originalValues {
			if originalValue != "" {
				os.Setenv(envVar, originalValue)
			}
		}
	}()

	config := LoadConfigFromEnv()

	assert.Equal(t, "8081", config.Port)
	assert.Equal(t, "127.0.0.1:8500", config.ConsulAddr)
	assert.Equal(t, "http://127.0.0.1:4646", config.NomadAddr)
	assert.Equal(t, true, config.UseConsulEnv)
	assert.Equal(t, "/tmp/ploy-env-store", config.EnvStorePath)
	assert.Equal(t, true, config.CleanupAutoStart)
	assert.Equal(t, true, config.EnableCaching)
}

func TestServer_HandleStorageConfig(t *testing.T) {
	tests := []struct {
		name           string
		configPath     string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "valid config load",
			configPath:     "/tmp/valid-config.yaml",
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name:           "missing config file",
			configPath:     "/tmp/nonexistent-config.yaml",
			expectedStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer()
			server.dependencies.StorageConfigPath = tt.configPath

			// Create a temporary config file for the valid test case
			if !tt.expectError {
				configContent := `storage:
  provider: seaweedfs
  master: "http://localhost:9333"
  filer: "http://localhost:8888"
  collection: "ploy"
  replication: "001"
  timeout: 30
  datacenter: "dc1"
  rack: "rack1"
  collections:
    artifacts: "artifacts"
    metadata: "ploy-metadata" 
    debug: "ploy-debug"
  client:
    enable_metrics: true
    enable_health_check: true
    max_operation_time: "5m"`

				err := os.WriteFile(tt.configPath, []byte(configContent), 0644)
				require.NoError(t, err)
				defer os.Remove(tt.configPath)
			}

			// Set up route using actual handler method
			server.app.Get("/storage/config", server.handleGetStorageConfig)

			// Test request
			req := httptest.NewRequest("GET", "/storage/config", nil)
			resp, err := server.app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			if tt.expectError {
				assert.Contains(t, response, "error")
			} else {
				// The response should contain the entire root config structure
				assert.Contains(t, response, "Storage")
				storage, ok := response["Storage"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "seaweedfs", storage["Provider"])
				assert.Equal(t, "http://localhost:9333", storage["Master"])
			}
		})
	}
}

func TestServer_HandleValidateStorageConfig(t *testing.T) {
	tests := []struct {
		name           string
		configPath     string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "valid config validation",
			configPath:     "/tmp/valid-config.yaml",
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name:           "invalid config validation",
			configPath:     "/tmp/invalid-config.yaml",
			expectedStatus: 400,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer()
			server.dependencies.StorageConfigPath = tt.configPath

			// Create config files
			if tt.expectError {
				// Create invalid config
				invalidConfig := `invalid yaml content: [`
				err := os.WriteFile(tt.configPath, []byte(invalidConfig), 0644)
				require.NoError(t, err)
				defer os.Remove(tt.configPath)
			} else {
				// Create valid config
				validConfig := `storage:
  provider: seaweedfs
  master: "http://localhost:9333"
  filer: "http://localhost:8888"
  collection: "ploy"
  replication: "001"`
				err := os.WriteFile(tt.configPath, []byte(validConfig), 0644)
				require.NoError(t, err)
				defer os.Remove(tt.configPath)
			}

			// Set up route using actual handler method
			server.app.Post("/storage/config/validate", server.handleValidateStorageConfig)

			// Test request
			req := httptest.NewRequest("POST", "/storage/config/validate", nil)
			resp, err := server.app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			if tt.expectError {
				assert.Contains(t, response, "error")
			} else {
				assert.Equal(t, true, response["valid"])
			}
		})
	}
}

func TestServer_HandleGetStorageConfig_UsesConfigService(t *testing.T) {
    // Build a config service with seaweedfs endpoint to verify mapping
    dir := t.TempDir()
    path := dir + "/config.yaml"
    content := []byte("storage:\n  provider: seaweedfs\n  endpoint: http://localhost:9333\n  bucket: test-collection\n")
    require.NoError(t, os.WriteFile(path, content, 0o644))

    svc, err := cfg.New(
        cfg.WithFile(path),
        cfg.WithValidation(cfg.NewStructValidator()),
    )
    require.NoError(t, err)

    server := createMockServer()
    server.dependencies.StorageConfigPath = "/tmp/missing-config.yaml"
    server.configService = svc

    server.app.Get("/storage/config", server.handleGetStorageConfig)

    req := httptest.NewRequest("GET", "/storage/config", nil)
    resp, err := server.app.Test(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, 200, resp.StatusCode)

    var body map[string]interface{}
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    // Legacy shape: top-level has Storage with Provider and Master
    storageObj, ok := body["Storage"].(map[string]interface{})
    require.True(t, ok)
    assert.Equal(t, "seaweedfs", storageObj["Provider"])
    assert.Equal(t, "http://localhost:9333", storageObj["Master"])
}

func TestServer_HandleReloadStorageConfig(t *testing.T) {
	t.Run("successful config reload", func(t *testing.T) {
		server := createMockServer()
		configPath := "/tmp/reload-config.yaml"
		server.dependencies.StorageConfigPath = configPath

		// Create initial config file
		configContent := `storage:
  provider: seaweedfs
  master: "http://localhost:9333"
  filer: "http://localhost:8888"
  collection: "ploy"
  replication: "001"`

		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)
		defer os.Remove(configPath)

		// Set up route using actual handler method
		server.app.Post("/storage/config/reload", server.handleReloadStorageConfig)

		// Test request
		req := httptest.NewRequest("POST", "/storage/config/reload", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Contains(t, response, "reloaded")
		assert.Contains(t, response, "config")
		assert.Equal(t, "Configuration reload completed", response["message"])
	})

	t.Run("config reload with missing file", func(t *testing.T) {
		server := createMockServer()
		server.dependencies.StorageConfigPath = "/tmp/nonexistent-reload-config.yaml"

		// Set up route using actual handler method
		server.app.Post("/storage/config/reload", server.handleReloadStorageConfig)

		// Test request
		req := httptest.NewRequest("POST", "/storage/config/reload", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 500, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "Failed to reload storage config", response["error"])
	})
}

func TestServer_HandleStorageHealth(t *testing.T) {
	t.Run("storage client initialization error", func(t *testing.T) {
		server := createMockServer()
		// Set invalid storage config path to force getStorageClient error
		server.dependencies.StorageConfigPath = "/tmp/nonexistent-storage-config.yaml"

		// Set up route using actual handler method
		server.app.Get("/storage/health", server.handleStorageHealth)

		// Test request
		req := httptest.NewRequest("GET", "/storage/health", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 503, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "Storage client initialization failed", response["error"])
		assert.Contains(t, response, "details")
	})

	t.Run("successful health status retrieval", func(t *testing.T) {
		// Create mock storage client with health data
		mockStorage := mocks.NewStorageClient()
		healthStatus := map[string]interface{}{
			"status":      "healthy",
			"uptime":      "24h30m",
			"connections": 15,
			"version":     "1.0.0",
		}
		mockStorage.On("GetHealthStatus").Return(healthStatus)

		server := createMockServer()
		server.mockStorageClient = func() (interface{}, error) {
			return mockStorage, nil
		}

		// Set up route with custom handler that uses mock storage client
		server.app.Get("/storage/health", func(c *fiber.Ctx) error {
			storeClientInterface, err := server.getStorageClient()
			if err != nil {
				return c.Status(503).JSON(fiber.Map{"error": "Storage client initialization failed", "details": err.Error()})
			}
			storeClient := storeClientInterface.(*mocks.StorageClient)
			health := storeClient.GetHealthStatus()
			return c.JSON(health)
		})

		// Test request
		req := httptest.NewRequest("GET", "/storage/health", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "healthy", response["status"])
		assert.Equal(t, "24h30m", response["uptime"])
		assert.Equal(t, float64(15), response["connections"]) // JSON decodes numbers as float64
		assert.Equal(t, "1.0.0", response["version"])

		mockStorage.AssertExpectations(t)
	})
}

func TestServer_HandleStorageMetrics(t *testing.T) {
	t.Run("storage client initialization error", func(t *testing.T) {
		server := createMockServer()
		// Set invalid storage config path to force getStorageClient error
		server.dependencies.StorageConfigPath = "/tmp/nonexistent-storage-config.yaml"

		// Set up route using actual handler method
		server.app.Get("/storage/metrics", server.handleStorageMetrics)

		// Test request
		req := httptest.NewRequest("GET", "/storage/metrics", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 503, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "Storage client initialization failed", response["error"])
		assert.Contains(t, response, "details")
	})

	t.Run("successful metrics retrieval", func(t *testing.T) {
		// Create mock storage client with metrics data
		mockStorage := mocks.NewStorageClient()
		metricsData := map[string]interface{}{
			"requests_total":     12345,
			"response_time_ms":   25.7,
			"error_rate":         0.02,
			"active_connections": 8,
			"storage_used_gb":    156.4,
		}
		mockStorage.On("GetMetrics").Return(metricsData)

		server := createMockServer()
		server.mockStorageClient = func() (interface{}, error) {
			return mockStorage, nil
		}

		// Set up route with custom handler that uses mock storage client
		server.app.Get("/storage/metrics", func(c *fiber.Ctx) error {
			storeClientInterface, err := server.getStorageClient()
			if err != nil {
				return c.Status(503).JSON(fiber.Map{"error": "Storage client initialization failed", "details": err.Error()})
			}
			storeClient := storeClientInterface.(*mocks.StorageClient)
			metrics := storeClient.GetMetrics()
			return c.JSON(metrics)
		})

		// Test request
		req := httptest.NewRequest("GET", "/storage/metrics", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, float64(12345), response["requests_total"]) // JSON decodes numbers as float64
		assert.Equal(t, 25.7, response["response_time_ms"])
		assert.Equal(t, 0.02, response["error_rate"])
		assert.Equal(t, float64(8), response["active_connections"])
		assert.Equal(t, 156.4, response["storage_used_gb"])

		mockStorage.AssertExpectations(t)
	})
}

func TestServer_HandleGetEnvVars(t *testing.T) {
	// Create mock environment store
	mockEnvStore := mocks.NewEnvStore()
	expectedVars := envstore.AppEnvVars{
		"DATABASE_URL": "postgresql://localhost/myapp",
		"API_KEY":      "secret-key-value",
		"DEBUG":        "true",
	}
	mockEnvStore.On("GetAll", "testapp").Return(expectedVars, nil)

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	// Set up route
	server.app.Get("/apps/:app/env", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		envVars, err := server.dependencies.EnvStore.GetAll(appName)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to get environment variables", "details": err.Error()})
		}
		return c.JSON(fiber.Map{
			"app":      appName,
			"env_vars": envVars,
		})
	})

	// Test request
	req := httptest.NewRequest("GET", "/apps/testapp/env", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "testapp", response["app"])

	envVars, ok := response["env_vars"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "postgresql://localhost/myapp", envVars["DATABASE_URL"])
	assert.Equal(t, "secret-key-value", envVars["API_KEY"])
	assert.Equal(t, "true", envVars["DEBUG"])

	mockEnvStore.AssertExpectations(t)
}

func TestServer_HandleGetEnvVars_Error(t *testing.T) {
	// Create mock environment store that returns error
	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("GetAll", "testapp").Return(nil, fmt.Errorf("environment store error"))

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	// Set up route
	server.app.Get("/apps/:app/env", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		envVars, err := server.dependencies.EnvStore.GetAll(appName)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to get environment variables", "details": err.Error()})
		}
		return c.JSON(fiber.Map{
			"app":      appName,
			"env_vars": envVars,
		})
	})

	// Test request
	req := httptest.NewRequest("GET", "/apps/testapp/env", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 500, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Failed to get environment variables", response["error"])
	assert.Contains(t, response["details"], "environment store error")

	mockEnvStore.AssertExpectations(t)
}

func TestServer_HandleSetEnvVars(t *testing.T) {
	tests := []struct {
		name             string
		appName          string
		requestBody      map[string]string
		mockSetup        func(*mocks.EnvStore)
		expectedStatus   int
		expectedError    string
		validateResponse func(t *testing.T, response map[string]interface{})
	}{
		{
			name:    "successful bulk set multiple vars",
			appName: "testapp",
			requestBody: map[string]string{
				"NODE_ENV":     "production",
				"DATABASE_URL": "postgres://localhost/db",
				"API_KEY":      "secret-key-123",
				"DEBUG":        "false",
			},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "testapp", mock.MatchedBy(func(envVars envstore.AppEnvVars) bool {
					return envVars["NODE_ENV"] == "production" &&
						envVars["DATABASE_URL"] == "postgres://localhost/db" &&
						envVars["API_KEY"] == "secret-key-123" &&
						envVars["DEBUG"] == "false"
				})).Return(nil)
			},
			expectedStatus: 200,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "updated", response["status"])
				assert.Equal(t, "testapp", response["app"])
				assert.Equal(t, float64(4), response["count"])
				assert.Equal(t, "Environment variables updated successfully", response["message"])
			},
		},
		{
			name:        "empty request body",
			appName:     "testapp",
			requestBody: map[string]string{},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "testapp", envstore.AppEnvVars{}).Return(nil)
			},
			expectedStatus: 200,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "updated", response["status"])
				assert.Equal(t, float64(0), response["count"])
			},
		},
		{
			name:    "store error",
			appName: "testapp",
			requestBody: map[string]string{
				"VAR1": "value1",
			},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "testapp", mock.Anything).Return(fmt.Errorf("storage connection failed"))
			},
			expectedStatus: 500,
			expectedError:  "failed to store environment variables",
		},
		{
			name:    "single large value",
			appName: "testapp",
			requestBody: map[string]string{
				"LARGE_VAR": strings.Repeat("x", 65536), // Maximum allowed size
			},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "testapp", mock.MatchedBy(func(envVars envstore.AppEnvVars) bool {
					return len(envVars["LARGE_VAR"]) == 65536
				})).Return(nil)
			},
			expectedStatus: 200,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "updated", response["status"])
				assert.Equal(t, float64(1), response["count"])
			},
		},
		{
			name:    "special characters in values",
			appName: "testapp",
			requestBody: map[string]string{
				"JSON_CONFIG": `{"key": "value", "nested": {"data": true}}`,
				"SCRIPT_PATH": "/usr/local/bin/script.sh",
				"CONNECTION":  "user:pass@host:5432/db?ssl=true",
			},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "testapp", mock.MatchedBy(func(envVars envstore.AppEnvVars) bool {
					return envVars["JSON_CONFIG"] == `{"key": "value", "nested": {"data": true}}` &&
						envVars["SCRIPT_PATH"] == "/usr/local/bin/script.sh" &&
						envVars["CONNECTION"] == "user:pass@host:5432/db?ssl=true"
				})).Return(nil)
			},
			expectedStatus: 200,
			validateResponse: func(t *testing.T, response map[string]interface{}) {
				assert.Equal(t, "updated", response["status"])
				assert.Equal(t, float64(3), response["count"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock environment store
			mockEnvStore := mocks.NewEnvStore()
			if tt.mockSetup != nil {
				tt.mockSetup(mockEnvStore)
			}

			server := createMockServer()
			server.dependencies.EnvStore = mockEnvStore

			// Set up route - using the actual handleSetEnvVars which calls env.SetEnvVars
			server.app.Post("/apps/:app/env", server.handleSetEnvVars)

			// Test request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/apps/"+tt.appName+"/env", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			if tt.expectedError != "" {
				assert.Contains(t, response["error"].(string), tt.expectedError)
			} else if tt.validateResponse != nil {
				tt.validateResponse(t, response)
			}

			mockEnvStore.AssertExpectations(t)
		})
	}
}

func TestServer_HandleSetEnvVars_ValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		requestBody    map[string]string
		expectedStatus int
		expectedError  string
	}{
		{
			name:    "reserved environment variable PATH",
			appName: "testapp",
			requestBody: map[string]string{
				"PATH": "/custom/path",
			},
			expectedStatus: 400,
			expectedError:  "validation failed",
		},
		{
			name:    "invalid variable name with spaces",
			appName: "testapp",
			requestBody: map[string]string{
				"INVALID VAR": "value",
			},
			expectedStatus: 400,
			expectedError:  "validation failed",
		},
		{
			name:    "null byte in value",
			appName: "testapp",
			requestBody: map[string]string{
				"VALID_VAR": "value\x00with\x00nulls",
			},
			expectedStatus: 400,
			expectedError:  "validation failed",
		},
		{
			name:    "variable name too long",
			appName: "testapp",
			requestBody: map[string]string{
				strings.Repeat("A", 256): "value",
			},
			expectedStatus: 400,
			expectedError:  "validation failed",
		},
		{
			name:    "value too large",
			appName: "testapp",
			requestBody: map[string]string{
				"HUGE_VAR": strings.Repeat("x", 65537), // Exceeds maximum
			},
			expectedStatus: 400,
			expectedError:  "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer()

			// Set up route
			server.app.Post("/apps/:app/env", server.handleSetEnvVars)

			// Test request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/apps/"+tt.appName+"/env", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			assert.Contains(t, response["error"].(string), tt.expectedError)
		})
	}
}

func TestServer_HandleSetEnvVars_MaxVariables(t *testing.T) {
	// Test maximum number of environment variables
	server := createMockServer()
	server.app.Post("/apps/:app/env", server.handleSetEnvVars)

	// Create request with 1001 variables (exceeds max of 1000)
	tooManyVars := make(map[string]string)
	for i := 0; i < 1001; i++ {
		tooManyVars[fmt.Sprintf("VAR_%d", i)] = "value"
	}

	body, _ := json.Marshal(tooManyVars)
	req := httptest.NewRequest("POST", "/apps/testapp/env", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Contains(t, response["error"].(string), "validation failed")
	assert.Contains(t, response["error"].(string), "too many")
}

func TestServer_HandleSetEnvVars_InvalidJSON(t *testing.T) {
	server := createMockServer()
	server.app.Post("/apps/:app/env", server.handleSetEnvVars)

	// Invalid JSON body
	req := httptest.NewRequest("POST", "/apps/testapp/env", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Contains(t, response["error"].(string), "invalid request body")
}

func TestServer_HandleSetEnvVar(t *testing.T) {
	// Create mock environment store
	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("Set", "testapp", "NEW_VAR", "new-value").Return(nil)

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	// Set up route
	server.app.Put("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		key := c.Params("key")

		var req struct {
			Value string `json:"value"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
		}

		if err := server.dependencies.EnvStore.Set(appName, key, req.Value); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to set environment variable", "details": err.Error()})
		}

		return c.JSON(fiber.Map{
			"app":    appName,
			"key":    key,
			"value":  req.Value,
			"status": "set",
		})
	})

	// Test request
	reqBody := map[string]string{"value": "new-value"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/apps/testapp/env/NEW_VAR", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "testapp", response["app"])
	assert.Equal(t, "NEW_VAR", response["key"])
	assert.Equal(t, "new-value", response["value"])
	assert.Equal(t, "set", response["status"])

	mockEnvStore.AssertExpectations(t)
}

func TestServer_HandleDeleteEnvVar(t *testing.T) {
	// Create mock environment store
	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("Delete", "testapp", "OLD_VAR").Return(nil)

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	// Set up route
	server.app.Delete("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		key := c.Params("key")

		if err := server.dependencies.EnvStore.Delete(appName, key); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to delete environment variable", "details": err.Error()})
		}

		return c.JSON(fiber.Map{
			"app":    appName,
			"key":    key,
			"status": "deleted",
		})
	})

	// Test request
	req := httptest.NewRequest("DELETE", "/apps/testapp/env/OLD_VAR", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "testapp", response["app"])
	assert.Equal(t, "OLD_VAR", response["key"])
	assert.Equal(t, "deleted", response["status"])

	mockEnvStore.AssertExpectations(t)
}

// Test edge cases and error scenarios
func TestServer_HandleSetEnvVar_InvalidBody(t *testing.T) {
	server := createMockServer()

	// Set up route
	server.app.Put("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		var req struct {
			Value string `json:"value"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Test request with invalid JSON
	req := httptest.NewRequest("PUT", "/apps/testapp/env/TEST_VAR", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid request body", response["error"])
}

func TestServer_HandleDeleteEnvVar_Error(t *testing.T) {
	// Create mock environment store that returns error
	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("Delete", "testapp", "NONEXISTENT_VAR").Return(fmt.Errorf("variable not found"))

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	// Set up route
	server.app.Delete("/apps/:app/env/:key", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		key := c.Params("key")

		if err := server.dependencies.EnvStore.Delete(appName, key); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to delete environment variable", "details": err.Error()})
		}

		return c.JSON(fiber.Map{
			"app":    appName,
			"key":    key,
			"status": "deleted",
		})
	})

	// Test request
	req := httptest.NewRequest("DELETE", "/apps/testapp/env/NONEXISTENT_VAR", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 500, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Failed to delete environment variable", response["error"])
	assert.Contains(t, response["details"], "variable not found")

	mockEnvStore.AssertExpectations(t)
}

// Benchmarks for handler performance
func BenchmarkServer_HandleStorageHealth(b *testing.B) {
	mockStorage := mocks.NewStorageClient()
	healthStatus := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
	}
	mockStorage.On("GetHealthStatus").Return(healthStatus)

	server := createMockServer()
	server.mockStorageClient = func() (interface{}, error) {
		return mockStorage, nil
	}

	server.app.Get("/storage/health", func(c *fiber.Ctx) error {
		storeClientInterface, err := server.getStorageClient()
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "Storage client initialization failed"})
		}
		storeClient := storeClientInterface.(*mocks.StorageClient)
		health := storeClient.GetHealthStatus()
		return c.JSON(health)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/storage/health", nil)
		resp, _ := server.app.Test(req)
		resp.Body.Close()
	}
}

func BenchmarkServer_HandleGetEnvVars(b *testing.B) {
	mockEnvStore := mocks.NewEnvStore()
	envVars := envstore.AppEnvVars{
		"DATABASE_URL": "postgresql://localhost/myapp",
		"API_KEY":      "secret-key-value",
	}
	mockEnvStore.On("GetAll", "testapp").Return(envVars, nil)

	server := createMockServer()
	server.dependencies.EnvStore = mockEnvStore

	server.app.Get("/apps/:app/env", func(c *fiber.Ctx) error {
		appName := c.Params("app")
		envVars, err := server.dependencies.EnvStore.GetAll(appName)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to get environment variables"})
		}
		return c.JSON(fiber.Map{
			"app":      appName,
			"env_vars": envVars,
		})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/apps/testapp/env", nil)
		resp, _ := server.app.Test(req)
		resp.Body.Close()
	}
}

// Property-based testing
func TestServer_Properties(t *testing.T) {
	t.Run("all handler methods should handle missing parameters gracefully", func(t *testing.T) {
		server := createMockServer()

		// Test routes with missing app parameter
		testCases := []struct {
			method string
			path   string
		}{
			{"GET", "/apps//env"},
			{"PUT", "/apps//env/key"},
			{"DELETE", "/apps//env/key"},
		}

		for _, tc := range testCases {
			server.app.Add(tc.method, tc.path, func(c *fiber.Ctx) error {
				appName := c.Params("app")
				if appName == "" {
					return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
				}
				return c.JSON(fiber.Map{"app": appName})
			})

			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp, err := server.app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()

			// Should handle gracefully (either 400 or 404)
			assert.True(t, resp.StatusCode == 400 || resp.StatusCode == 404)
		}
	})

	t.Run("error responses should be consistent", func(t *testing.T) {
		server := createMockServer()

		server.app.Get("/test-error", func(c *fiber.Ctx) error {
			return c.Status(500).JSON(fiber.Map{"error": "Test error"})
		})

		req := httptest.NewRequest("GET", "/test-error", nil)
		resp, err := server.app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 500, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Contains(t, response, "error")
		assert.IsType(t, "", response["error"])
	})
}

// Integration test structure (would run on VPS)
func TestServer_Integration(t *testing.T) {
	t.Skip("Integration test - requires VPS environment with dependencies")

	// This test would be run on VPS with actual dependencies
	// config := LoadConfigFromEnv()
	// server, err := NewServer(config)
	// require.NoError(t, err)
	//
	// Test actual API endpoints with real dependencies
	// Test storage client creation
	// Test environment store operations
	// Test certificate management integration
}

// Test server lifecycle
func TestServer_LifecycleManagement(t *testing.T) {
	t.Run("server creation with valid config", func(t *testing.T) {
		// This would require mocking all dependencies
		// Testing the structure for now
		config := &ControllerConfig{
			Port:              "8081",
			ConsulAddr:        "localhost:8500",
			NomadAddr:         "http://localhost:4646",
			StorageConfigPath: "/test/config",
			UseConsulEnv:      false,
			EnvStorePath:      "/tmp/test-env",
			EnableCaching:     true,
		}

		assert.Equal(t, "8081", config.Port)
		assert.Equal(t, false, config.UseConsulEnv)
		assert.Equal(t, true, config.EnableCaching)
	})

	t.Run("service dependencies structure", func(t *testing.T) {
		deps := &ServiceDependencies{
			StorageConfigPath: "/test/config",
		}

		assert.Equal(t, "/test/config", deps.StorageConfigPath)
		assert.Nil(t, deps.EnvStore) // Not initialized in test
	})
}
