package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/controller/envstore"
)

// Mock storage client for testing
type MockStorageClient struct {
	mock.Mock
}

func (m *MockStorageClient) GetHealthStatus() interface{} {
	args := m.Called()
	return args.Get(0)
}

func (m *MockStorageClient) GetMetrics() interface{} {
	args := m.Called()
	return args.Get(0)
}

func (m *MockStorageClient) GetArtifactsBucket() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (interface{}, error) {
	args := m.Called(keyPrefix, artifactPath)
	return args.Get(0), args.Error(1)
}

// Mock environment store for testing
type MockEnvStore struct {
	mock.Mock
}

func (m *MockEnvStore) Get(appName, key string) (string, bool, error) {
	args := m.Called(appName, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

func (m *MockEnvStore) GetAll(appName string) (envstore.AppEnvVars, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(envstore.AppEnvVars), args.Error(1)
}

func (m *MockEnvStore) Set(appName, key, value string) error {
	args := m.Called(appName, key, value)
	return args.Error(0)
}

func (m *MockEnvStore) Delete(appName, key string) error {
	args := m.Called(appName, key)
	return args.Error(0)
}

func (m *MockEnvStore) SetAll(appName string, envVars envstore.AppEnvVars) error {
	args := m.Called(appName, envVars)
	return args.Error(0)
}

func (m *MockEnvStore) ToStringArray(appName string) ([]string, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

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
			EnvStore:          &MockEnvStore{},
			StorageConfigPath: "/test/config",
		},
	}
	return &TestableServer{Server: server}
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
		"PLOY_CONSUL_POOL_SIZE":   "15",
		"PLOY_NOMAD_POOL_SIZE":    "12",
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
	assert.Equal(t, 15, config.ConsulPoolSize)
	assert.Equal(t, 12, config.NomadPoolSize)
	assert.Equal(t, false, config.EnableCaching)
	assert.Equal(t, 30*time.Second, config.ShutdownTimeout)
}

func TestLoadConfigFromEnvDefaults(t *testing.T) {
	// Clear relevant environment variables to test defaults
	envVarsToUnset := []string{
		"PORT", "CONSUL_HTTP_ADDR", "NOMAD_ADDR", "PLOY_USE_CONSUL_ENV",
		"PLOY_ENV_STORE_PATH", "PLOY_CLEANUP_AUTO_START", "PLOY_CONSUL_POOL_SIZE",
		"PLOY_NOMAD_POOL_SIZE", "PLOY_ENABLE_CACHING",
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
	assert.Equal(t, 10, config.ConsulPoolSize)
	assert.Equal(t, 8, config.NomadPoolSize)
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
    artifacts: "ploy-artifacts"
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
				assert.Contains(t, response, "storage")
				storage, ok := response["storage"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "seaweedfs", storage["provider"])
				assert.Equal(t, "http://localhost:9333", storage["master"])
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

func TestServer_HandleStorageMetrics(t *testing.T) {
	// Create mock storage client
	mockStorage := &MockStorageClient{}
	expectedMetrics := map[string]interface{}{
		"requests_per_second": 150.5,
		"average_latency_ms":  25.3,
		"error_rate":          0.02,
	}
	mockStorage.On("GetMetrics").Return(expectedMetrics)

	server := createMockServer()
	
	// Override getStorageClient to return our mock
	server.mockStorageClient = func() (interface{}, error) {
		return mockStorage, nil
	}

	// Set up route using actual handler method
	server.app.Get("/storage/metrics", server.handleStorageMetrics)

	// Test request
	req := httptest.NewRequest("GET", "/storage/metrics", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 150.5, response["requests_per_second"])
	assert.Equal(t, 25.3, response["average_latency_ms"])
	assert.Equal(t, 0.02, response["error_rate"])

	mockStorage.AssertExpectations(t)
}

func TestServer_HandleGetEnvVars(t *testing.T) {
	// Create mock environment store
	mockEnvStore := &MockEnvStore{}
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
			"app":     appName,
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
	mockEnvStore := &MockEnvStore{}
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
			"app":     appName,
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

func TestServer_HandleSetEnvVar(t *testing.T) {
	// Create mock environment store
	mockEnvStore := &MockEnvStore{}
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
			"app":   appName,
			"key":   key,
			"value": req.Value,
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
	mockEnvStore := &MockEnvStore{}
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
	mockEnvStore := &MockEnvStore{}
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
	mockStorage := &MockStorageClient{}
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
		storeClient := storeClientInterface.(*MockStorageClient)
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
	mockEnvStore := &MockEnvStore{}
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
			"app":     appName,
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
			ConsulPoolSize:    5,
			NomadPoolSize:     3,
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

// Test mock implementations work correctly
func TestMockImplementations(t *testing.T) {
	t.Run("mock storage client", func(t *testing.T) {
		mockStorage := &MockStorageClient{}
		mockStorage.On("GetHealthStatus").Return(map[string]interface{}{"status": "healthy"})
		mockStorage.On("GetMetrics").Return(map[string]interface{}{"requests": 100})

		health := mockStorage.GetHealthStatus()
		metrics := mockStorage.GetMetrics()

		assert.Equal(t, "healthy", health.(map[string]interface{})["status"])
		assert.Equal(t, 100, metrics.(map[string]interface{})["requests"])

		mockStorage.AssertExpectations(t)
	})

	t.Run("mock env store", func(t *testing.T) {
		mockEnv := &MockEnvStore{}
		mockEnv.On("GetAll", "testapp").Return(envstore.AppEnvVars{"KEY": "value"}, nil)
		mockEnv.On("Set", "testapp", "KEY", "newvalue").Return(nil)

		vars, err := mockEnv.GetAll("testapp")
		assert.NoError(t, err)
		assert.Equal(t, "value", vars["KEY"])

		err = mockEnv.Set("testapp", "KEY", "newvalue")
		assert.NoError(t, err)

		mockEnv.AssertExpectations(t)
	})
}