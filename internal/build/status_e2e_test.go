//go:build e2e
// +build e2e

package build

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Status function validation logic under e2e tag
func TestStatus(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		expectedStatus int
		expectedError  string
		skipReason     string
	}{
		{
			name:           "invalid app name - too short",
			appName:        "x",
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:           "invalid app name - special characters",
			appName:        "invalid@app",
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:           "invalid app name - reserved name",
			appName:        "api",
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:       "valid app name - nomad integration required",
			appName:    "valid-app",
			skipReason: "Integration test - requires Nomad API for job status lookup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}
			app := fiber.New()
			app.Get("/apps/:app/status", Status)
			url := "/apps/" + tt.appName + "/status"
			req := httptest.NewRequest("GET", url, nil)
			resp, err := app.Test(req, 10000)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			if tt.expectedError != "" {
				var responseBody map[string]interface{}
				_ = json.NewDecoder(resp.Body).Decode(&responseBody)
				if errorMsg, exists := responseBody["error"]; exists {
					assert.Contains(t, errorMsg.(string), tt.expectedError)
				}
			}
		})
	}
}
