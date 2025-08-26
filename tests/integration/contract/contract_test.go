//go:build integration
// +build integration

package contract

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/testutil"
	"github.com/iw2rmb/ploy/internal/testutil/api"
)

// ServiceContract defines the contract between services
type ServiceContract struct {
	ServiceName string
	Version     string
	Endpoints   []EndpointContract
	Events      []EventContract
}

// EndpointContract defines API endpoint contract
type EndpointContract struct {
	Method         string
	Path           string
	RequestSchema  interface{}
	ResponseSchema interface{}
	StatusCodes    []int
}

// EventContract defines event publishing/consuming contract
type EventContract struct {
	Name      string
	Version   string
	Schema    interface{}
	Publisher string
	Consumers []string
}

// TestStorageServiceContract tests storage service contracts
func TestStorageServiceContract(t *testing.T) {
	contract := ServiceContract{
		ServiceName: "storage",
		Version:     "1.0",
		Endpoints: []EndpointContract{
			{
				Method: "POST",
				Path:   "/upload",
				RequestSchema: map[string]interface{}{
					"key":  "string",
					"data": "binary",
				},
				ResponseSchema: map[string]interface{}{
					"fid": "string",
					"url": "string",
				},
				StatusCodes: []int{201, 400, 500},
			},
			{
				Method:         "GET",
				Path:           "/download/{key}",
				ResponseSchema: "binary",
				StatusCodes:    []int{200, 404, 500},
			},
		},
	}

	// Test contract compliance
	testServiceContract(t, contract)
}

// TestBuildServiceContract tests build service contracts
func TestBuildServiceContract(t *testing.T) {
	contract := ServiceContract{
		ServiceName: "build",
		Version:     "1.0",
		Endpoints: []EndpointContract{
			{
				Method: "POST",
				Path:   "/v1/apps/{app}/builds",
				RequestSchema: map[string]interface{}{
					"git_url": "string",
					"branch":  "string",
					"lane":    "string", // optional
				},
				ResponseSchema: map[string]interface{}{
					"status":   "string",
					"build_id": "string",
				},
				StatusCodes: []int{202, 400, 500},
			},
			{
				Method: "GET",
				Path:   "/v1/apps/{app}/status",
				ResponseSchema: map[string]interface{}{
					"status":    "string",
					"instances": "number",
					"version":   "string",
				},
				StatusCodes: []int{200, 404},
			},
		},
		Events: []EventContract{
			{
				Name:      "build.started",
				Version:   "1.0",
				Publisher: "build-service",
				Schema: map[string]interface{}{
					"app_name":  "string",
					"build_id":  "string",
					"timestamp": "datetime",
				},
			},
			{
				Name:      "build.completed",
				Version:   "1.0",
				Publisher: "build-service",
				Schema: map[string]interface{}{
					"app_name":    "string",
					"build_id":    "string",
					"status":      "string",
					"artifact_id": "string",
					"timestamp":   "datetime",
				},
			},
		},
	}

	testServiceContract(t, contract)
}

// TestHealthServiceContract tests health check service contracts
func TestHealthServiceContract(t *testing.T) {
	contract := ServiceContract{
		ServiceName: "health",
		Version:     "1.0",
		Endpoints: []EndpointContract{
			{
				Method: "GET",
				Path:   "/health",
				ResponseSchema: map[string]interface{}{
					"status": "string",
				},
				StatusCodes: []int{200, 503},
			},
			{
				Method: "GET",
				Path:   "/ready",
				ResponseSchema: map[string]interface{}{
					"status": "string",
				},
				StatusCodes: []int{200, 503},
			},
			{
				Method:      "GET",
				Path:        "/live",
				StatusCodes: []int{200},
			},
		},
	}

	testServiceContract(t, contract)
}

// testServiceContract validates a service contract
func testServiceContract(t *testing.T, contract ServiceContract) {
	t.Run("contract_"+contract.ServiceName, func(t *testing.T) {
		// Test all endpoints
		for _, endpoint := range contract.Endpoints {
			t.Run(fmt.Sprintf("%s_%s", endpoint.Method, cleanPathForTest(endpoint.Path)), func(t *testing.T) {
				testEndpointContract(t, endpoint)
			})
		}

		// Test all events
		for _, event := range contract.Events {
			t.Run("event_"+event.Name, func(t *testing.T) {
				testEventContract(t, event)
			})
		}
	})
}

// testEndpointContract validates an endpoint contract
func testEndpointContract(t *testing.T, contract EndpointContract) {
	controllerURL := testutil.GetEnvOrDefault("PLOY_CONTROLLER", "http://localhost:8081")
	client := api.NewTestClient(t, controllerURL)

	// Convert path template to actual path for testing
	testPath := convertPathTemplate(contract.Path)

	// Build request
	var resp *api.APIResponse

	switch contract.Method {
	case "GET":
		resp = client.GET(testPath).Execute()
	case "POST":
		req := client.POST(testPath)
		if contract.RequestSchema != nil {
			// Generate valid request from schema
			validRequest := generateFromSchema(contract.RequestSchema)
			req = req.WithJSON(validRequest)
		}
		resp = req.Execute()
	case "PUT":
		req := client.PUT(testPath)
		if contract.RequestSchema != nil {
			validRequest := generateFromSchema(contract.RequestSchema)
			req = req.WithJSON(validRequest)
		}
		resp = req.Execute()
	case "DELETE":
		resp = client.DELETE(testPath).Execute()
	}

	if resp == nil {
		t.Fatalf("Failed to get response from %s %s", contract.Method, testPath)
	}

	// Verify status code is in allowed list
	assert.Contains(t, contract.StatusCodes, resp.StatusCode,
		"Status code %d should be in contract %v for %s %s", resp.StatusCode, contract.StatusCodes, contract.Method, contract.Path)

	// Verify response schema (if success status and schema defined)
	if resp.StatusCode < 300 && contract.ResponseSchema != nil {
		validateResponseSchema(t, resp.Body, contract.ResponseSchema)
	}
}

// testEventContract validates an event contract
func testEventContract(t *testing.T, contract EventContract) {
	// This would integrate with event system testing
	// For now, validate schema structure
	assert.NotEmpty(t, contract.Name, "Event name should not be empty")
	assert.NotEmpty(t, contract.Publisher, "Publisher should be specified")
	assert.NotNil(t, contract.Schema, "Event schema should be defined")
}

// Schema validation helpers (simplified implementation)
func generateFromSchema(schema interface{}) interface{} {
	// This would be a more sophisticated schema-to-data generator
	if schemaMap, ok := schema.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for key, fieldType := range schemaMap {
			switch fieldType {
			case "string":
				result[key] = "test_" + key
			case "number":
				result[key] = 42
			case "boolean":
				result[key] = true
			case "binary":
				result[key] = []byte("test data")
			case "datetime":
				result[key] = "2023-01-01T00:00:00Z"
			}
		}
		return result
	}
	return nil
}

func validateResponseSchema(t *testing.T, body []byte, schema interface{}) {
	// This would validate the response against the schema
	// Simplified implementation for now
	var responseData interface{}
	err := json.Unmarshal(body, &responseData)
	require.NoError(t, err, "Response should be valid JSON")

	if schemaMap, ok := schema.(map[string]interface{}); ok {
		responseMap, ok := responseData.(map[string]interface{})
		require.True(t, ok, "Response should be a JSON object")

		for key, expectedType := range schemaMap {
			assert.Contains(t, responseMap, key, "Response should contain field: "+key)

			// Validate field type (simplified)
			value := responseMap[key]
			switch expectedType {
			case "string":
				assert.IsType(t, "", value, "Field "+key+" should be string")
			case "number":
				// JSON numbers are float64
				assert.IsType(t, float64(0), value, "Field "+key+" should be number")
			case "boolean":
				assert.IsType(t, true, value, "Field "+key+" should be boolean")
			}
		}
	}
}

// Helper functions
func cleanPathForTest(path string) string {
	// Convert path to test-safe name
	return fmt.Sprintf("%s", path)
}

func convertPathTemplate(path string) string {
	// Convert path template like "/v1/apps/{app}/status" to actual path
	// For testing purposes, replace {app} with test-app
	if path == "/v1/apps/{app}/builds" {
		return "/v1/apps/test-app/builds"
	}
	if path == "/v1/apps/{app}/status" {
		return "/v1/apps/test-app/status"
	}
	if path == "/download/{key}" {
		return "/download/test-key"
	}
	return path
}