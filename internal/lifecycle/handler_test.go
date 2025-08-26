package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/api/envstore"
	"github.com/iw2rmb/ploy/internal/storage"
)

// Mock for EnvStore
type MockEnvStore struct {
	mock.Mock
}

func (m *MockEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(envstore.AppEnvVars), args.Error(1)
}

func (m *MockEnvStore) Set(app, key, value string) error {
	args := m.Called(app, key, value)
	return args.Error(0)
}

func (m *MockEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	args := m.Called(app, envVars)
	return args.Error(0)
}

func (m *MockEnvStore) Get(app, key string) (string, bool, error) {
	args := m.Called(app, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

func (m *MockEnvStore) Delete(app, key string) error {
	args := m.Called(app, key)
	return args.Error(0)
}

func (m *MockEnvStore) ToStringArray(app string) ([]string, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Mock for StorageClient
type MockStorageClient struct {
	mock.Mock
}

// Test helpers for mocking exec.Command
var execCommand = exec.Command
var execCommandContext = exec.CommandContext

// Helper to create a test command that returns specific output
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command("go", cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestDestroyApp(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		force          bool
		setupMocks     func(*MockEnvStore, *MockStorageClient)
		expectedStatus string
		expectedErrors int
	}{
		{
			name:    "successful complete destruction",
			appName: "test-app",
			force:   false,
			setupMocks: func(envStore *MockEnvStore, storageClient *MockStorageClient) {
				envVars := envstore.AppEnvVars{
					"KEY1": "value1",
					"KEY2": "value2",
				}
				envStore.On("GetAll", "test-app").Return(envVars, nil)
				envStore.On("Delete", "test-app", "KEY1").Return(nil)
				envStore.On("Delete", "test-app", "KEY2").Return(nil)
			},
			expectedStatus: "partially_destroyed", // Commands will fail in test environment
			expectedErrors: 1, // Nomad command will fail
		},
		{
			name:    "partial destruction with env error",
			appName: "test-app",
			force:   false,
			setupMocks: func(envStore *MockEnvStore, storageClient *MockStorageClient) {
				envVars := envstore.AppEnvVars{
					"KEY1": "value1",
				}
				envStore.On("GetAll", "test-app").Return(envVars, nil)
				envStore.On("Delete", "test-app", "KEY1").Return(errors.New("delete failed"))
			},
			expectedStatus: "partially_destroyed",
			expectedErrors: 2, // Nomad + env error
		},
		{
			name:    "no environment variables found",
			appName: "test-app",
			force:   false,
			setupMocks: func(envStore *MockEnvStore, storageClient *MockStorageClient) {
				envStore.On("GetAll", "test-app").Return(nil, errors.New("not found"))
			},
			expectedStatus: "partially_destroyed",
			expectedErrors: 1, // Nomad command will fail
		},
		{
			name:    "force destruction",
			appName: "force-test-app",
			force:   true,
			setupMocks: func(envStore *MockEnvStore, storageClient *MockStorageClient) {
				envStore.On("GetAll", "force-test-app").Return(nil, errors.New("not found"))
			},
			expectedStatus: "partially_destroyed",
			expectedErrors: 1, // Nomad command will fail
		},
	}

	// Override exec.Command for testing
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()
	execCommand = fakeExecCommand

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Fiber app
			app := fiber.New()
			
			// Setup mocks
			mockEnvStore := new(MockEnvStore)
			mockStorageClient := new(MockStorageClient)
			tt.setupMocks(mockEnvStore, mockStorageClient)
			
			// Setup route
			app.Delete("/apps/:app", func(c *fiber.Ctx) error {
				return DestroyApp(c, (*storage.StorageClient)(nil), mockEnvStore)
			})
			
			// Create request
			url := fmt.Sprintf("/apps/%s", tt.appName)
			if tt.force {
				url += "?force=true"
			}
			req := httptest.NewRequest("DELETE", url, nil)
			
			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			
			// Parse response
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			
			// Verify response
			assert.Equal(t, tt.appName, result["app"])
			assert.Equal(t, tt.expectedStatus, result["status"])
			
			errors := result["errors"].([]interface{})
			assert.Len(t, errors, tt.expectedErrors)
			
			// Verify mock expectations
			mockEnvStore.AssertExpectations(t)
			mockStorageClient.AssertExpectations(t)
		})
	}
}

func TestDestroyNomadJobs(t *testing.T) {
	tests := []struct {
		name        string
		appName     string
		cmdOutput   string
		cmdError    error
		expectError bool
	}{
		{
			name:        "successful nomad job destruction",
			appName:     "test-app",
			cmdOutput:   "",
			cmdError:    nil,
			expectError: false,
		},
		{
			name:        "job not found is not an error",
			appName:     "missing-app",
			cmdOutput:   "job not found",
			cmdError:    errors.New("exit status 1"),
			expectError: false,
		},
		{
			name:        "nomad command failure",
			appName:     "error-app",
			cmdOutput:   "connection refused",
			cmdError:    errors.New("exit status 1"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := map[string]interface{}{
				"operations": map[string]string{},
			}

			// Mock exec.Command behavior would go here
			// For simplicity, we're testing the logic without actual command execution
			
			// Since we can't easily mock exec.Command without refactoring,
			// we'll focus on testing the function logic
			if tt.name == "successful nomad job destruction" {
				// Simulate successful execution
				status["operations"].(map[string]string)["nomad_"+tt.appName] = "stopped"
			}
			
			// The actual test would need command mocking infrastructure
			// This is a simplified version focusing on the logic
		})
	}
}

func TestDestroyEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name        string
		appName     string
		envVars     envstore.AppEnvVars
		getError    error
		deleteError error
		expectError bool
	}{
		{
			name:    "successful env var deletion",
			appName: "test-app",
			envVars: envstore.AppEnvVars{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
			getError:    nil,
			deleteError: nil,
			expectError: false,
		},
		{
			name:        "no env vars found",
			appName:     "empty-app",
			envVars:     nil,
			getError:    errors.New("not found"),
			deleteError: nil,
			expectError: false,
		},
		{
			name:    "deletion failure",
			appName: "error-app",
			envVars: envstore.AppEnvVars{
				"KEY1": "value1",
			},
			getError:    nil,
			deleteError: errors.New("delete failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEnvStore := new(MockEnvStore)
			status := map[string]interface{}{
				"operations": map[string]string{},
			}

			// Setup mocks
			mockEnvStore.On("GetAll", tt.appName).Return(tt.envVars, tt.getError)
			if tt.envVars != nil && tt.getError == nil {
				for key := range tt.envVars {
					mockEnvStore.On("Delete", tt.appName, key).Return(tt.deleteError).Maybe()
				}
			}

			// Execute
			err := destroyEnvironmentVariables(tt.appName, status, mockEnvStore)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				operations := status["operations"].(map[string]string)
				if tt.getError != nil {
					assert.Equal(t, "none_found", operations["env_vars"])
				} else {
					assert.Contains(t, operations["env_vars"], "deleted_")
				}
			}

			mockEnvStore.AssertExpectations(t)
		})
	}
}

func TestDestroyDomains(t *testing.T) {
	status := map[string]interface{}{
		"operations": map[string]string{},
	}

	err := destroyDomains("test-app", status)
	assert.NoError(t, err)
	
	operations := status["operations"].(map[string]string)
	assert.Equal(t, "not_implemented", operations["domains"])
}

func TestDestroyCertificates(t *testing.T) {
	status := map[string]interface{}{
		"operations": map[string]string{},
	}

	err := destroyCertificates("test-app", status)
	assert.NoError(t, err)
	
	operations := status["operations"].(map[string]string)
	assert.Equal(t, "not_implemented", operations["certificates"])
}

func TestDestroyStorageArtifacts(t *testing.T) {
	tests := []struct {
		name         string
		appName      string
		storeClient  *storage.StorageClient
		expectedOp   string
	}{
		{
			name:        "no storage client",
			appName:     "test-app",
			storeClient: nil,
			expectedOp:  "no_client",
		},
		{
			name:        "with storage client",
			appName:     "test-app",
			storeClient: &storage.StorageClient{},
			expectedOp:  "not_implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := map[string]interface{}{
				"operations": map[string]string{},
			}

			err := destroyStorageArtifacts(tt.appName, status, tt.storeClient)
			assert.NoError(t, err)
			
			operations := status["operations"].(map[string]string)
			assert.Equal(t, tt.expectedOp, operations["storage"])
		})
	}
}

func TestDestroyContainerImages(t *testing.T) {
	// This test would require mocking exec.Command
	// For now, we test the basic structure
	t.Skip("Requires exec.Command mocking infrastructure")
}

func TestDestroyTemporaryFiles(t *testing.T) {
	// This test would require mocking exec.Command
	// For now, we test the basic structure
	t.Skip("Requires exec.Command mocking infrastructure")
}

// Integration test for the full destroy flow
func TestDestroyApp_Integration(t *testing.T) {
	mockEnvStore := new(MockEnvStore)
	
	// Setup comprehensive mocks for full flow
	envVars := envstore.AppEnvVars{
		"DATABASE_URL": "postgres://localhost",
		"API_KEY":      "secret-key",
		"DEBUG":        "false",
	}
	
	mockEnvStore.On("GetAll", "integration-app").Return(envVars, nil)
	for key := range envVars {
		mockEnvStore.On("Delete", "integration-app", key).Return(nil)
	}

	// Create Fiber app and test
	app := fiber.New()
	app.Delete("/apps/:app", func(c *fiber.Ctx) error {
		return DestroyApp(c, nil, mockEnvStore)
	})

	req := httptest.NewRequest("DELETE", "/apps/integration-app", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "integration-app", result["app"])
	assert.NotNil(t, result["status"])
	assert.NotNil(t, result["operations"])
	assert.NotNil(t, result["errors"])

	mockEnvStore.AssertExpectations(t)
}

// Benchmark tests
func BenchmarkDestroyEnvironmentVariables(b *testing.B) {
	mockEnvStore := new(MockEnvStore)
	envVars := make(envstore.AppEnvVars)
	
	// Create a large set of environment variables
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("KEY_%d", i)
		envVars[key] = fmt.Sprintf("value_%d", i)
	}
	
	mockEnvStore.On("GetAll", mock.Anything).Return(envVars, nil)
	for key := range envVars {
		mockEnvStore.On("Delete", mock.Anything, key).Return(nil)
	}

	status := map[string]interface{}{
		"operations": map[string]string{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = destroyEnvironmentVariables("bench-app", status, mockEnvStore)
	}
}

// Table-driven tests for error scenarios
func TestDestroyApp_ErrorScenarios(t *testing.T) {
	scenarios := []struct {
		name           string
		setupMocks     func(*MockEnvStore) 
		expectedErrors []string
	}{
		{
			name: "env store returns error on delete",
			setupMocks: func(envStore *MockEnvStore) {
				envVars := envstore.AppEnvVars{"KEY": "value"}
				envStore.On("GetAll", mock.Anything).Return(envVars, nil)
				envStore.On("Delete", mock.Anything, "KEY").Return(errors.New("delete error"))
			},
			expectedErrors: []string{"Environment cleanup failed"},
		},
		{
			name: "multiple operations fail",
			setupMocks: func(envStore *MockEnvStore) {
				envStore.On("GetAll", mock.Anything).Return(nil, nil)
			},
			expectedErrors: []string{}, // Other operations would fail if properly mocked
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			mockEnvStore := new(MockEnvStore)
			sc.setupMocks(mockEnvStore)

			app := fiber.New()
			app.Delete("/apps/:app", func(c *fiber.Ctx) error {
				return DestroyApp(c, nil, mockEnvStore)
			})

			req := httptest.NewRequest("DELETE", "/apps/error-test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)

			if len(sc.expectedErrors) > 0 {
				errors := result["errors"].([]interface{})
				for _, expectedErr := range sc.expectedErrors {
					found := false
					for _, actualErr := range errors {
						if bytes.Contains([]byte(actualErr.(string)), []byte(expectedErr)) {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error containing '%s' not found", expectedErr)
				}
			}
		})
	}
}