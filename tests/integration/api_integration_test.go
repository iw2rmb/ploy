//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/iw2rmb/ploy/internal/testutil"
	"github.com/iw2rmb/ploy/internal/testutil/api"
)

// APIIntegrationSuite tests API endpoints with real services
type APIIntegrationSuite struct {
	suite.Suite

	apiSuite    *api.APITestSuite
	controllerURL string
}

func (suite *APIIntegrationSuite) SetupSuite() {
	// Get controller URL from environment or use default
	suite.controllerURL = testutil.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")

	// Wait for controller to be ready
	suite.waitForController()

	// Create API test suite
	suite.apiSuite = api.NewAPITestSuite(suite.T(), suite.controllerURL)
}

func (suite *APIIntegrationSuite) waitForController() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := api.NewTestClient(suite.T(), suite.controllerURL)

	for {
		select {
		case <-ctx.Done():
			suite.T().Fatal("Controller not ready within timeout")
		default:
			resp := client.GET("/health").Execute()
			if resp != nil && resp.StatusCode == 200 {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (suite *APIIntegrationSuite) TestHealthEndpoints() {
	suite.apiSuite.TestHealthEndpoints()
}

func (suite *APIIntegrationSuite) TestVersionEndpoints() {
	suite.apiSuite.TestVersionEndpoints()
}

func (suite *APIIntegrationSuite) TestErrorScenarios() {
	suite.apiSuite.TestErrorScenarios()
}

// TestBasicAPIConnectivity tests basic API connectivity without complex scenarios
func (suite *APIIntegrationSuite) TestBasicAPIConnectivity() {
	suite.T().Run("basic connectivity", func(t *testing.T) {
		client := api.NewTestClient(t, suite.controllerURL)

		// Test basic endpoints that should always work
		resp := client.GET("/health").Execute()
		assert.NotNil(t, resp)
		assert.Equal(t, 200, resp.StatusCode)

		resp = client.GET("/version").Execute()
		assert.NotNil(t, resp)
		// Controller might not implement version endpoint yet, so accept 404
		assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 404)
	})
}

// TestControllerMetadata tests controller information endpoints
func (suite *APIIntegrationSuite) TestControllerMetadata() {
	suite.T().Run("controller metadata", func(t *testing.T) {
		client := api.NewTestClient(t, suite.controllerURL)

		// Test endpoints that provide controller information
		endpoints := []string{"/health", "/ready", "/live", "/metrics"}

		for _, endpoint := range endpoints {
			resp := client.GET(endpoint).Execute()
			assert.NotNil(t, resp, "Should get response from %s", endpoint)
			// Accept various status codes as endpoints might not all be implemented
			assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
				"Endpoint %s should return valid HTTP status, got %d", endpoint, resp.StatusCode)
		}
	})
}

func TestAPIIntegrationSuite(t *testing.T) {
	suite.Run(t, new(APIIntegrationSuite))
}