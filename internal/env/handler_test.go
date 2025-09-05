package env

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/testing/mocks"
)

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

func createTestRequest(method, path string, body interface{}) *http.Request {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// Tests for SetEnvVars function
func TestSetEnvVars(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		requestBody    interface{}
		mockSetup      func(*mocks.EnvStore)
		expectedStatus int
		expectedFields map[string]interface{}
		wantErr        bool
	}{
		{
			name:    "successful environment variables set",
			appName: "test-app",
			requestBody: map[string]string{
				"DATABASE_URL": "postgresql://localhost/testdb",
				"API_KEY":      "secret-key-123",
				"DEBUG":        "true",
			},
			mockSetup: func(store *mocks.EnvStore) {
				expectedVars := envstore.AppEnvVars{
					"DATABASE_URL": "postgresql://localhost/testdb",
					"API_KEY":      "secret-key-123",
					"DEBUG":        "true",
				}
				store.On("SetAll", "test-app", expectedVars).Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "updated",
				"app":     "test-app",
				"count":   float64(3), // JSON numbers decode as float64
				"message": "Environment variables updated successfully",
			},
			wantErr: false,
		},
		{
			name:           "invalid JSON body",
			appName:        "test-app",
			requestBody:    "invalid-json",
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "invalid request body",
			},
			wantErr: true,
		},
		{
			name:    "env store error",
			appName: "test-app",
			requestBody: map[string]string{
				"TEST_VAR": "test-value",
			},
			mockSetup: func(store *mocks.EnvStore) {
				expectedVars := envstore.AppEnvVars{
					"TEST_VAR": "test-value",
				}
				store.On("SetAll", "test-app", expectedVars).Return(fmt.Errorf("storage error"))
			},
			expectedStatus: 500,
			expectedFields: map[string]interface{}{
				"error": "failed to store environment variables: storage error",
			},
			wantErr: true,
		},
		{
			name:    "invalid environment variable name with spaces",
			appName: "test-app",
			requestBody: map[string]string{
				"INVALID VAR": "value",
			},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "validation failed: invalid environment variable 'INVALID VAR': environment variable name contains invalid character (space)",
			},
			wantErr: true,
		},
		{
			name:    "reserved environment variable name",
			appName: "test-app",
			requestBody: map[string]string{
				"PATH": "/custom/path",
			},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "validation failed: invalid environment variable 'PATH': environment variable name 'PATH' is reserved and cannot be modified",
			},
			wantErr: true,
		},
		{
			name:    "environment variable value with null byte",
			appName: "test-app",
			requestBody: map[string]string{
				"VALID_VAR": "value\x00with\x00null",
			},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "validation failed: invalid value for environment variable 'VALID_VAR': environment variable value contains null byte",
			},
			wantErr: true,
		},
		{
			name:        "empty environment variables",
			appName:     "test-app",
			requestBody: map[string]string{},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("SetAll", "test-app", envstore.AppEnvVars{}).Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "updated",
				"app":     "test-app",
				"count":   float64(0),
				"message": "Environment variables updated successfully",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock environment store
			mockStore := mocks.NewEnvStore()
			tt.mockSetup(mockStore)

			// Create test app and route
			app := createTestApp()
			app.Post("/apps/:app/env", func(c *fiber.Ctx) error {
				return SetEnvVars(c, mockStore)
			})

			// Create test request
			var req *http.Request
			if tt.requestBody == "invalid-json" {
				req = httptest.NewRequest("POST", "/apps/"+tt.appName+"/env", bytes.NewReader([]byte("invalid-json")))
			} else {
				req = createTestRequest("POST", "/apps/"+tt.appName+"/env", tt.requestBody)
			}

			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Verify response body
			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)

			// Check expected fields
			for key, expectedValue := range tt.expectedFields {
				assert.Equal(t, expectedValue, responseBody[key], "Field %s should match expected value", key)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

// Tests for GetEnvVars function
func TestGetEnvVars(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		mockSetup      func(*mocks.EnvStore)
		expectedStatus int
		expectedFields map[string]interface{}
		wantErr        bool
	}{
		{
			name:    "successful environment variables retrieval",
			appName: "test-app",
			mockSetup: func(store *mocks.EnvStore) {
				envVars := envstore.AppEnvVars{
					"DATABASE_URL": "postgresql://localhost/testdb",
					"API_KEY":      "secret-key-123",
					"DEBUG":        "false",
				}
				store.On("GetAll", "test-app").Return(envVars, nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"app": "test-app",
			},
			wantErr: false,
		},
		{
			name:    "empty environment variables",
			appName: "empty-app",
			mockSetup: func(store *mocks.EnvStore) {
				envVars := envstore.AppEnvVars{}
				store.On("GetAll", "empty-app").Return(envVars, nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"app": "empty-app",
			},
			wantErr: false,
		},
		{
			name:    "env store error",
			appName: "error-app",
			mockSetup: func(store *mocks.EnvStore) {
				store.On("GetAll", "error-app").Return(nil, fmt.Errorf("storage connection error"))
			},
			expectedStatus: 500,
			expectedFields: map[string]interface{}{
				"error": "failed to retrieve environment variables: storage connection error",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock environment store
			mockStore := mocks.NewEnvStore()
			tt.mockSetup(mockStore)

			// Create test app and route
			app := createTestApp()
			app.Get("/apps/:app/env", func(c *fiber.Ctx) error {
				return GetEnvVars(c, mockStore)
			})

			// Create test request
			req := createTestRequest("GET", "/apps/"+tt.appName+"/env", nil)

			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Verify response body
			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)

			// Check expected fields
			for key, expectedValue := range tt.expectedFields {
				assert.Equal(t, expectedValue, responseBody[key], "Field %s should match expected value", key)
			}

			// For successful cases, verify env vars structure
			if !tt.wantErr {
				assert.Contains(t, responseBody, "env", "Response should contain 'env' field")
				envVars, ok := responseBody["env"].(map[string]interface{})
				require.True(t, ok, "env field should be a map")

				// Verify specific env vars for the first test case
				if tt.name == "successful environment variables retrieval" {
					assert.Equal(t, "postgresql://localhost/testdb", envVars["DATABASE_URL"])
					assert.Equal(t, "secret-key-123", envVars["API_KEY"])
					assert.Equal(t, "false", envVars["DEBUG"])
				}
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

// Tests for SetEnvVar function
func TestSetEnvVar(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		keyName        string
		requestBody    interface{}
		mockSetup      func(*mocks.EnvStore)
		expectedStatus int
		expectedFields map[string]interface{}
		wantErr        bool
	}{
		{
			name:        "successful environment variable set",
			appName:     "test-app",
			keyName:     "NEW_VAR",
			requestBody: map[string]string{"value": "test-value-123"},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Set", "test-app", "NEW_VAR", "test-value-123").Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "updated",
				"app":     "test-app",
				"key":     "NEW_VAR",
				"message": "Environment variable updated successfully",
			},
			wantErr: false,
		},
		{
			name:           "invalid JSON body",
			appName:        "test-app",
			keyName:        "TEST_VAR",
			requestBody:    "invalid-json",
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "invalid request body",
			},
			wantErr: true,
		},
		{
			name:        "env store error",
			appName:     "test-app",
			keyName:     "ERROR_VAR",
			requestBody: map[string]string{"value": "error-value"},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Set", "test-app", "ERROR_VAR", "error-value").Return(fmt.Errorf("storage write error"))
			},
			expectedStatus: 500,
			expectedFields: map[string]interface{}{
				"error": "failed to store environment variable: storage write error",
			},
			wantErr: true,
		},
		{
			name:           "invalid variable name with special char",
			appName:        "test-app",
			keyName:        "INVALID-VAR",
			requestBody:    map[string]string{"value": "value"},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "invalid environment variable name: environment variable name contains invalid character '-'",
			},
			wantErr: true,
		},
		{
			name:           "reserved variable name PATH",
			appName:        "test-app",
			keyName:        "PATH",
			requestBody:    map[string]string{"value": "/custom/path"},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "invalid environment variable name: environment variable name 'PATH' is reserved and cannot be modified",
			},
			wantErr: true,
		},
		{
			name:           "value with null byte",
			appName:        "test-app",
			keyName:        "VALID_VAR",
			requestBody:    map[string]string{"value": "value\x00null"},
			mockSetup:      func(store *mocks.EnvStore) {},
			expectedStatus: 400,
			expectedFields: map[string]interface{}{
				"error": "invalid environment variable value: environment variable value contains null byte",
			},
			wantErr: true,
		},
		{
			name:        "empty value",
			appName:     "test-app",
			keyName:     "EMPTY_VAR",
			requestBody: map[string]string{"value": ""},
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Set", "test-app", "EMPTY_VAR", "").Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "updated",
				"app":     "test-app",
				"key":     "EMPTY_VAR",
				"message": "Environment variable updated successfully",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock environment store
			mockStore := mocks.NewEnvStore()
			tt.mockSetup(mockStore)

			// Create test app and route
			app := createTestApp()
			app.Put("/apps/:app/env/:key", func(c *fiber.Ctx) error {
				return SetEnvVar(c, mockStore)
			})

			// Create test request
			var req *http.Request
			if tt.requestBody == "invalid-json" {
				req = httptest.NewRequest("PUT", "/apps/"+tt.appName+"/env/"+url.PathEscape(tt.keyName), bytes.NewReader([]byte("invalid-json")))
			} else {
				req = createTestRequest("PUT", "/apps/"+tt.appName+"/env/"+url.PathEscape(tt.keyName), tt.requestBody)
			}

			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Verify response body
			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)

			// Check expected fields
			for key, expectedValue := range tt.expectedFields {
				assert.Equal(t, expectedValue, responseBody[key], "Field %s should match expected value", key)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

// Tests for DeleteEnvVar function
func TestDeleteEnvVar(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		keyName        string
		mockSetup      func(*mocks.EnvStore)
		expectedStatus int
		expectedFields map[string]interface{}
		wantErr        bool
	}{
		{
			name:    "successful environment variable deletion",
			appName: "test-app",
			keyName: "OLD_VAR",
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Delete", "test-app", "OLD_VAR").Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "deleted",
				"app":     "test-app",
				"key":     "OLD_VAR",
				"message": "Environment variable deleted successfully",
			},
			wantErr: false,
		},
		{
			name:    "env store error",
			appName: "test-app",
			keyName: "NONEXISTENT_VAR",
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Delete", "test-app", "NONEXISTENT_VAR").Return(fmt.Errorf("variable not found"))
			},
			expectedStatus: 500,
			expectedFields: map[string]interface{}{
				"error": "failed to delete environment variable: variable not found",
			},
			wantErr: true,
		},
		{
			name:    "deletion of special characters key",
			appName: "test-app",
			keyName: "SPECIAL_VAR-123",
			mockSetup: func(store *mocks.EnvStore) {
				store.On("Delete", "test-app", "SPECIAL_VAR-123").Return(nil)
			},
			expectedStatus: 200,
			expectedFields: map[string]interface{}{
				"status":  "deleted",
				"app":     "test-app",
				"key":     "SPECIAL_VAR-123",
				"message": "Environment variable deleted successfully",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock environment store
			mockStore := mocks.NewEnvStore()
			tt.mockSetup(mockStore)

			// Create test app and route
			app := createTestApp()
			app.Delete("/apps/:app/env/:key", func(c *fiber.Ctx) error {
				return DeleteEnvVar(c, mockStore)
			})

			// Create test request
			req := createTestRequest("DELETE", "/apps/"+tt.appName+"/env/"+tt.keyName, nil)

			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Verify response body
			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)

			// Check expected fields
			for key, expectedValue := range tt.expectedFields {
				assert.Equal(t, expectedValue, responseBody[key], "Field %s should match expected value", key)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}
