package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/iw2rmb/ploy/internal/testutil"
)

// APITestSuite provides comprehensive API testing scenarios
type APITestSuite struct {
	client   *TestClient
	fixtures *testutil.TestDataRepository
	t        *testing.T
}

// NewAPITestSuite creates a new API test suite
func NewAPITestSuite(t *testing.T, baseURL string) *APITestSuite {
	return &APITestSuite{
		client:   NewTestClient(t, baseURL),
		fixtures: testutil.NewTestDataRepository(),
		t:        t,
	}
}

// TestHealthEndpoints verifies all health check endpoints
func (suite *APITestSuite) TestHealthEndpoints() {
	suite.t.Run("health endpoints", func(t *testing.T) {
		// Basic health check
		suite.client.GET("/health").
			ExpectStatus(200).
			ExpectJSON().
			Execute().
			AssertJSONPath("status", "healthy")

		// Readiness check
		suite.client.GET("/ready").
			ExpectStatus(200).
			ExpectJSON().
			Execute()

		// Liveness check
		suite.client.GET("/live").
			ExpectStatus(200).
			Execute()

		// Metrics endpoint
		suite.client.GET("/metrics").
			ExpectStatus(200).
			Execute()
	})
}

// TestAppLifecycle tests complete application lifecycle
func (suite *APITestSuite) TestAppLifecycle() {
	suite.t.Run("app lifecycle", func(t *testing.T) {
		appName := "test-lifecycle-app"

		// 1. Create app via build trigger
		buildRequest := map[string]interface{}{
			"git_url": "https://github.com/test/go-app.git",
			"branch":  "main",
		}

		suite.client.POST("/v1/apps/"+appName+"/builds").
			WithJSON(buildRequest).
			ExpectStatus(202).
			ExpectJSON().
			Execute().
			AssertJSONPath("status", "build_triggered")

		// 2. Check app appears in list
		suite.client.GET("/v1/apps").
			ExpectStatus(200).
			ExpectJSON().
			Execute()
		// Could add more specific assertions about app in list

		// 3. Get app status
		suite.client.GET("/v1/apps/"+appName+"/status").
			ExpectStatus(200).
			ExpectJSON().
			Execute()

		// 4. Set environment variables
		envVars := map[string]string{
			"NODE_ENV": "test",
			"DEBUG":    "true",
		}

		suite.client.POST("/v1/apps/"+appName+"/env").
			WithJSON(envVars).
			ExpectStatus(200).
			Execute()

		// 5. Get environment variables
		suite.client.GET("/v1/apps/"+appName+"/env").
			ExpectStatus(200).
			ExpectJSON().
			Execute().
			AssertJSONPath("env.NODE_ENV", "test").
			AssertJSONPath("env.DEBUG", "true")

		// 6. Cleanup - destroy app
		suite.client.DELETE("/v1/apps/"+appName).
			ExpectStatus(200).
			Execute()

		// 7. Verify app is gone
		suite.client.GET("/v1/apps/"+appName+"/status").
			ExpectStatus(404).
			Execute()
	})
}

// TestEnvironmentVariables tests comprehensive env var management
func (suite *APITestSuite) TestEnvironmentVariables() {
	suite.t.Run("environment variables", func(t *testing.T) {
		appName := "test-env-app"

		// Initially empty
		suite.client.GET("/v1/apps/"+appName+"/env").
			ExpectStatus(200).
			ExpectJSON().
			Execute().
			AssertJSONPath("env", map[string]interface{}{})

		// Set multiple variables
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		}

		suite.client.POST("/v1/apps/"+appName+"/env").
			WithJSON(envVars).
			ExpectStatus(200).
			Execute()

		// Verify all set
		resp := suite.client.GET("/v1/apps/"+appName+"/env").
			ExpectStatus(200).
			ExpectJSON().
			Execute()

		var envResponse map[string]interface{}
		resp.JSON(&envResponse)

		envMap := envResponse["env"].(map[string]interface{})
		assert.Equal(suite.t, "value1", envMap["VAR1"])
		assert.Equal(suite.t, "value2", envMap["VAR2"])
		assert.Equal(suite.t, "value3", envMap["VAR3"])

		// Update single variable
		suite.client.PUT("/v1/apps/"+appName+"/env/VAR1").
			WithJSON(map[string]string{"value": "updated_value1"}).
			ExpectStatus(200).
			Execute()

		// Verify update
		suite.client.GET("/v1/apps/"+appName+"/env").
			ExpectStatus(200).
			Execute().
			AssertJSONPath("env.VAR1", "updated_value1").
			AssertJSONPath("env.VAR2", "value2") // unchanged

		// Delete single variable
		suite.client.DELETE("/v1/apps/"+appName+"/env/VAR2").
			ExpectStatus(200).
			Execute()

		// Verify deletion
		suite.client.GET("/v1/apps/"+appName+"/env").
			ExpectStatus(200).
			Execute().
			AssertJSONPath("env.VAR1", "updated_value1")
		// VAR2 should not exist
		// VAR3 should still exist
	})
}

// TestDomainManagement tests domain management endpoints
func (suite *APITestSuite) TestDomainManagement() {
	suite.t.Run("domain management", func(t *testing.T) {
		appName := "test-domain-app"
		domain := "test.example.com"

		// Add domain
		suite.client.POST("/v1/apps/"+appName+"/domains").
			WithJSON(map[string]string{"domain": domain}).
			ExpectStatus(201).
			Execute()

		// List domains
		suite.client.GET("/v1/apps/"+appName+"/domains").
			ExpectStatus(200).
			ExpectJSON().
			Execute()

		// Remove domain
		suite.client.DELETE("/v1/apps/"+appName+"/domains/"+domain).
			ExpectStatus(200).
			Execute()

		// Verify domain removed
		suite.client.GET("/v1/apps/"+appName+"/domains").
			ExpectStatus(200).
			ExpectJSON().
			Execute()
		// Should verify empty domain list
	})
}

// TestVersionEndpoints tests version and info endpoints
func (suite *APITestSuite) TestVersionEndpoints() {
	suite.t.Run("version endpoints", func(t *testing.T) {
		// Version endpoint
		suite.client.GET("/version").
			ExpectStatus(200).
			ExpectJSON().
			Execute().
			AssertJSONPath("version", "dev")

		// Build info endpoint
		suite.client.GET("/v1/build-info").
			ExpectStatus(200).
			ExpectJSON().
			Execute()
	})
}

// TestErrorScenarios tests various error conditions
func (suite *APITestSuite) TestErrorScenarios() {
	suite.t.Run("error scenarios", func(t *testing.T) {
		// Non-existent app
		suite.client.GET("/v1/apps/non-existent-app/status").
			ExpectStatus(404).
			Execute()

		// Invalid JSON payload
		suite.client.POST("/v1/apps/test-app/env").
			WithHeader("Content-Type", "application/json").
			WithJSON("invalid-json").
			ExpectStatus(400).
			Execute()

		// Invalid app name
		suite.client.POST("/v1/apps/invalid@app#name/builds").
			WithJSON(map[string]string{"git_url": "https://github.com/test/app.git"}).
			ExpectStatus(400).
			Execute()

		// Missing required fields
		suite.client.POST("/v1/apps/test-app/builds").
			WithJSON(map[string]string{}).
			ExpectStatus(400).
			Execute()
	})
}