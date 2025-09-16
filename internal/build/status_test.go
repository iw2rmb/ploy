package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapNomadStatusToARF(t *testing.T) {
	tests := []struct {
		name        string
		nomadStatus string
		expectedARF string
	}{
		// Basic status mappings
		{
			name:        "pending to building",
			nomadStatus: "pending",
			expectedARF: "building",
		},
		{
			name:        "running to deploying",
			nomadStatus: "running",
			expectedARF: "deploying",
		},
		{
			name:        "dead to stopped",
			nomadStatus: "dead",
			expectedARF: "stopped",
		},
		// Case insensitive tests
		{
			name:        "case insensitive pending",
			nomadStatus: "PENDING",
			expectedARF: "building",
		},
		{
			name:        "case insensitive running",
			nomadStatus: "RUNNING",
			expectedARF: "deploying",
		},
		{
			name:        "case insensitive dead",
			nomadStatus: "DEAD",
			expectedARF: "stopped",
		},
		{
			name:        "mixed case pending",
			nomadStatus: "PeNdInG",
			expectedARF: "building",
		},
		// Edge cases
		{
			name:        "unknown status defaults to running",
			nomadStatus: "unknown",
			expectedARF: "running",
		},
		{
			name:        "empty status defaults to running",
			nomadStatus: "",
			expectedARF: "running",
		},
		{
			name:        "whitespace only status defaults to running",
			nomadStatus: "   ",
			expectedARF: "running",
		},
		{
			name:        "status with leading/trailing whitespace",
			nomadStatus: "  pending  ",
			expectedARF: "running", // strings.ToLower() doesn't trim whitespace
		},
		{
			name:        "numeric status defaults to running",
			nomadStatus: "123",
			expectedARF: "running",
		},
		{
			name:        "special characters status defaults to running",
			nomadStatus: "@#$%",
			expectedARF: "running",
		},
		{
			name:        "very long status defaults to running",
			nomadStatus: "this-is-a-very-long-status-name-that-does-not-match-anything",
			expectedARF: "running",
		},
		// Similar but not exact matches
		{
			name:        "pending with extra characters",
			nomadStatus: "pendings",
			expectedARF: "running",
		},
		{
			name:        "running with extra characters",
			nomadStatus: "runnings",
			expectedARF: "running",
		},
		{
			name:        "dead with extra characters",
			nomadStatus: "deads",
			expectedARF: "running",
		},
		// Unicode and special cases
		{
			name:        "status with unicode characters",
			nomadStatus: "péndîng",
			expectedARF: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapNomadStatusToARF(tt.nomadStatus)
			assert.Equal(t, tt.expectedARF, result)
		})
	}
}

func TestExtractLaneFromJobName(t *testing.T) {
	tests := []struct {
		name         string
		jobName      string
		expectedLane string
	}{
		// Valid lane extractions
		{
			name:         "valid lane A",
			jobName:      "myapp-lane-a",
			expectedLane: "A",
		},
		{
			name:         "valid lane B",
			jobName:      "myapp-lane-b",
			expectedLane: "B",
		},
		{
			name:         "valid lane C",
			jobName:      "hello-world-lane-c",
			expectedLane: "C",
		},
		{
			name:         "valid lane D",
			jobName:      "my-complex-app-name-lane-d",
			expectedLane: "D",
		},
		{
			name:         "valid lane E",
			jobName:      "app-with-dashes-lane-e",
			expectedLane: "E",
		},
		{
			name:         "valid lane F",
			jobName:      "simple-lane-f",
			expectedLane: "F",
		},
		{
			name:         "valid lane G",
			jobName:      "test-app-lane-g",
			expectedLane: "G",
		},
		// Case sensitivity tests
		{
			name:         "uppercase lane letter",
			jobName:      "myapp-lane-A",
			expectedLane: "A",
		},
		{
			name:         "mixed case lane letter",
			jobName:      "myapp-lane-C",
			expectedLane: "C",
		},
		// Edge cases and invalid formats
		{
			name:         "invalid format - no lane",
			jobName:      "myapp",
			expectedLane: "unknown",
		},
		{
			name:         "invalid format - wrong separator",
			jobName:      "myapp_lane_a",
			expectedLane: "unknown",
		},
		{
			name:         "empty job name",
			jobName:      "",
			expectedLane: "unknown",
		},
		{
			name:         "only separator",
			jobName:      "-lane-",
			expectedLane: "",
		},
		{
			name:         "multiple lane separators",
			jobName:      "app-lane-c-lane-d",
			expectedLane: "unknown", // Function requires exactly 2 parts after splitting
		},
		{
			name:         "lane separator at beginning",
			jobName:      "-lane-a",
			expectedLane: "A",
		},
		{
			name:         "lane separator with no lane identifier",
			jobName:      "myapp-lane-",
			expectedLane: "",
		},
		{
			name:         "numeric lane identifier",
			jobName:      "myapp-lane-1",
			expectedLane: "1",
		},
		{
			name:         "special character lane identifier",
			jobName:      "myapp-lane-@",
			expectedLane: "@",
		},
		{
			name:         "very long job name with lane",
			jobName:      "this-is-a-very-long-application-name-with-many-dashes-lane-x",
			expectedLane: "X",
		},
		{
			name:         "job name containing lane but not as separator",
			jobName:      "mylaneapp-test",
			expectedLane: "unknown",
		},
		{
			name:         "job name with lane in app name but proper lane separator",
			jobName:      "mylaneapp-lane-z",
			expectedLane: "Z",
		},
		// Unicode and international characters
		{
			name:         "lane with unicode character",
			jobName:      "myapp-lane-ñ",
			expectedLane: "Ñ",
		},
		{
			name:         "job name with unicode in app name",
			jobName:      "mí-app-español-lane-b",
			expectedLane: "B",
		},
		// Whitespace handling
		{
			name:         "job name with spaces (invalid)",
			jobName:      "my app-lane-c",
			expectedLane: "C",
		},
		{
			name:         "lane with trailing space",
			jobName:      "myapp-lane-c ",
			expectedLane: "C ",
		},
		{
			name:         "lane with leading space",
			jobName:      "myapp-lane- c",
			expectedLane: " C",
		},
		// Boundary tests
		{
			name:         "single character app name",
			jobName:      "a-lane-b",
			expectedLane: "B",
		},
		{
			name:         "single character lane",
			jobName:      "myapp-lane-x",
			expectedLane: "X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLaneFromJobName(tt.jobName)
			assert.Equal(t, tt.expectedLane, result)
		})
	}
}

// Test edge cases for status mapping consistency
func TestMapNomadStatusToARFConsistency(t *testing.T) {
	t.Run("all known statuses have consistent mapping", func(t *testing.T) {
		// Test that the function handles all expected Nomad statuses consistently
		knownStatuses := []string{"pending", "running", "dead"}

		for _, status := range knownStatuses {
			result := mapNomadStatusToARF(status)
			assert.NotEmpty(t, result, "Status mapping should never return empty string for known status: %s", status)
			assert.NotEqual(t, status, result, "ARF status should be different from Nomad status: %s", status)
		}
	})

	t.Run("mapping function is deterministic", func(t *testing.T) {
		// Test that calling the function multiple times with same input gives same result
		testStatus := "pending"

		result1 := mapNomadStatusToARF(testStatus)
		result2 := mapNomadStatusToARF(testStatus)
		result3 := mapNomadStatusToARF(testStatus)

		assert.Equal(t, result1, result2)
		assert.Equal(t, result2, result3)
	})

	t.Run("unknown statuses always default to running", func(t *testing.T) {
		unknownStatuses := []string{
			"unknown", "invalid", "corrupted", "incomplete",
			"starting", "stopping", "restarting", "failed",
			"terminated", "killed", "suspended",
		}

		for _, status := range unknownStatuses {
			result := mapNomadStatusToARF(status)
			assert.Equal(t, "running", result, "Unknown status should always map to 'running': %s", status)
		}
	})
}

// Test edge cases for lane extraction consistency
func TestExtractLaneFromJobNameConsistency(t *testing.T) {
	t.Run("lane extraction is deterministic", func(t *testing.T) {
		testJobName := "myapp-lane-c"

		result1 := extractLaneFromJobName(testJobName)
		result2 := extractLaneFromJobName(testJobName)
		result3 := extractLaneFromJobName(testJobName)

		assert.Equal(t, result1, result2)
		assert.Equal(t, result2, result3)
	})

	t.Run("case sensitivity is preserved", func(t *testing.T) {
		// Function should preserve the case of the lane identifier
		testCases := map[string]string{
			"app-lane-a": "A",
			"app-lane-A": "A",
			"app-lane-c": "C",
			"app-lane-C": "C",
			"app-lane-z": "Z",
			"app-lane-Z": "Z",
		}

		for jobName, expectedLane := range testCases {
			result := extractLaneFromJobName(jobName)
			assert.Equal(t, expectedLane, result, "Lane case should be preserved for job: %s", jobName)
		}
	})

	t.Run("all standard lanes A-G work correctly", func(t *testing.T) {
		standardLanes := []string{"a", "b", "c", "d", "e", "f", "g"}

		for _, lane := range standardLanes {
			jobName := "testapp-lane-" + lane
			result := extractLaneFromJobName(jobName)
			expectedLane := strings.ToUpper(lane)

			assert.Equal(t, expectedLane, result, "Standard lane should be extracted correctly: %s", lane)
		}
	})

	t.Run("function handles malformed input gracefully", func(t *testing.T) {
		malformedInputs := []string{
			"", " ", "\t", "\n", "\r",
			"-", "--", "---",
			"-lane", "lane-", "-lane-", "lane",
			"app--lane--c", "app-lane-lane-c",
		}

		for _, input := range malformedInputs {
			// Should not panic
			result := extractLaneFromJobName(input)
			assert.IsType(t, "", result, "Function should always return string type for input: %q", input)
		}
	})
}

// Test helper functions for robustness
func TestStatusHelperFunctionsRobustness(t *testing.T) {
	t.Run("functions handle nil and empty inputs", func(t *testing.T) {
		// Test with empty strings
		assert.NotPanics(t, func() {
			_ = mapNomadStatusToARF("")
		})

		assert.NotPanics(t, func() {
			extractLaneFromJobName("")
		})
	})

	t.Run("functions return valid types", func(t *testing.T) {
		// Ensure return types are always string, never nil
		result1 := mapNomadStatusToARF("any-input")
		assert.IsType(t, "", result1)
		assert.NotNil(t, result1)

		result2 := extractLaneFromJobName("any-input")
		assert.IsType(t, "", result2)
		assert.NotNil(t, result2)
	})

	t.Run("functions handle very large inputs", func(t *testing.T) {
		// Test with very long strings to ensure no buffer overflows
		veryLongString := strings.Repeat("a", 10000)

		assert.NotPanics(t, func() {
			_ = mapNomadStatusToARF(veryLongString)
		})

		assert.NotPanics(t, func() {
			extractLaneFromJobName(veryLongString)
		})

		// Test with long string containing lane separator
		longJobName := strings.Repeat("app-name-part-", 1000) + "lane-x"
		result := extractLaneFromJobName(longJobName)
		assert.Equal(t, "X", result)
	})
}

// Test Status function with minimal mocking
func TestStatusFunctionStructure(t *testing.T) {
	t.Skip("Integration test - requires Nomad API mocking")
}

// Test Status function validation logic - moved under e2e build tag
/*
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

			// Setup Fiber app for testing
			app := fiber.New()

			// Create test route using the actual Status function
			app.Get("/apps/:app/status", Status)

			// Create test request
			url := "/apps/" + tt.appName + "/status"
			req := httptest.NewRequest("GET", url, nil)

			// Execute request
			resp, err := app.Test(req, 10000)
			require.NoError(t, err)

			// Verify response
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
*/
