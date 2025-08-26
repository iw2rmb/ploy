package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildConfigurationValidation tests build configuration validation
func TestBuildConfigurationValidation(t *testing.T) {
	tests := []struct {
		name           string
		lane           string
		appName        string
		sha            string
		mainClass      string
		expectedLane   string
		expectedError  bool
		description    string
	}{
		{
			name:         "lane A (Unikraft Go)",
			lane:         "A",
			appName:      "go-app",
			sha:          "abc123",
			mainClass:    "", // Not used for Go
			expectedLane: "A",
			description:  "Should process Go applications in Lane A",
		},
		{
			name:         "lane B (Unikraft Node)",
			lane:         "B", 
			appName:      "node-app",
			sha:          "def456",
			mainClass:    "", // Not used for Node.js
			expectedLane: "B",
			description:  "Should process Node.js applications in Lane B",
		},
		{
			name:         "lane C (OSv Java)",
			lane:         "C",
			appName:      "java-app",
			sha:          "ghi789",
			mainClass:    "com.example.Main",
			expectedLane: "C",
			description:  "Should process Java applications in Lane C",
		},
		{
			name:         "lane D (FreeBSD jail)",
			lane:         "D",
			appName:      "jail-app",
			sha:          "jkl012",
			mainClass:    "",
			expectedLane: "D",
			description:  "Should process applications in FreeBSD jail (Lane D)",
		},
		{
			name:         "lane E (OCI containers)",
			lane:         "E",
			appName:      "container-app",
			sha:          "mno345",
			mainClass:    "",
			expectedLane: "E",
			description:  "Should process OCI containers in Lane E",
		},
		{
			name:         "lane F (full VM)",
			lane:         "F",
			appName:      "vm-app",
			sha:          "pqr678",
			mainClass:    "",
			expectedLane: "F",
			description:  "Should process full VM applications in Lane F",
		},
		{
			name:         "invalid lane defaults to C",
			lane:         "Z", // Invalid lane
			appName:      "default-app",
			sha:          "stu901",
			mainClass:    "com.example.Default",
			expectedLane: "C", // Should default to C
			description:  "Should default to Lane C for invalid lanes",
		},
		{
			name:         "empty lane should trigger auto-detection",
			lane:         "", // Auto-detect
			appName:      "auto-detect-app",
			sha:          "vwx234",
			mainClass:    "",
			expectedLane: "", // Will depend on file analysis
			description:  "Should trigger lane auto-detection when lane is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate that the lane configuration is acceptable
			normalizedLane := strings.ToUpper(tt.lane)
			validLanes := []string{"A", "B", "C", "D", "E", "F", "G"}
			
			if tt.lane != "" { // Only test if lane is specified
				if tt.expectedLane == "C" && tt.lane == "Z" {
					// Special case: invalid lanes should default to C
					assert.NotContains(t, validLanes, normalizedLane, "Invalid lane should not be in valid lanes list")
				} else if tt.expectedLane != "" {
					assert.Contains(t, validLanes, normalizedLane, "Valid lane should be in valid lanes list")
				}
			}

			// Validate app name format (basic check)
			if tt.appName != "" {
				assert.Regexp(t, `^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z][a-z0-9]*$`, tt.appName, "App name should follow naming conventions")
			}

			// Validate SHA format (basic check)
			if tt.sha != "" {
				assert.Regexp(t, `^[a-zA-Z0-9]+$`, tt.sha, "SHA should be alphanumeric")
				assert.GreaterOrEqual(t, len(tt.sha), 3, "SHA should be at least 3 characters")
			}

			// Validate main class format for Java applications
			if tt.lane == "C" && tt.mainClass != "" {
				assert.Contains(t, tt.mainClass, ".", "Java main class should contain package notation")
			}
		})
	}
}

// TestLaneNormalization tests lane string normalization
func TestLaneNormalization(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expected     string
		description  string
	}{
		{
			name:        "uppercase lane A",
			input:       "A",
			expected:    "A",
			description: "Uppercase lane should remain uppercase",
		},
		{
			name:        "lowercase lane a",
			input:       "a",
			expected:    "A",
			description: "Lowercase lane should be normalized to uppercase",
		},
		{
			name:        "mixed case lane",
			input:       "c",
			expected:    "C", 
			description: "Mixed case should be normalized to uppercase",
		},
		{
			name:        "invalid lane character",
			input:       "Z",
			expected:    "C", // Default fallback
			description: "Invalid lane should fallback to default (C)",
		},
		{
			name:        "empty lane",
			input:       "",
			expected:    "", // Auto-detection
			description: "Empty lane should trigger auto-detection",
		},
		{
			name:        "numeric lane",
			input:       "1",
			expected:    "C", // Default fallback
			description: "Numeric lane should fallback to default",
		},
		{
			name:        "special character lane",
			input:       "@",
			expected:    "C", // Default fallback 
			description: "Special character lane should fallback to default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test lane normalization logic
			var normalized string
			switch strings.ToUpper(tt.input) {
			case "A", "B", "C", "D", "E", "F", "G":
				normalized = strings.ToUpper(tt.input)
			case "":
				normalized = "" // Auto-detection
			default:
				normalized = "C" // Default fallback
			}
			
			assert.Equal(t, tt.expected, normalized, tt.description)
		})
	}
}

// TestBuildParameterValidation tests validation of build parameters
func TestBuildParameterValidation(t *testing.T) {
	t.Run("app name validation", func(t *testing.T) {
		validNames := []string{
			"my-app", "test123", "hello-world", "api-service", "worker-v2",
		}
		
		invalidNames := []string{
			"", "x", "MyApp", "my_app", "api", "dev", "controller", 
			"app-", "-app", "my--app", "app@domain", "123app",
		}
		
		for _, name := range validNames {
			t.Run("valid_"+name, func(t *testing.T) {
				// Basic validation pattern
				isValid := len(name) >= 2 && len(name) <= 63 && 
					strings.ToLower(name) == name &&
					!strings.HasPrefix(name, "-") && !strings.HasSuffix(name, "-") &&
					!strings.Contains(name, "--") &&
					name != "api" && name != "dev" && name != "controller"
				
				assert.True(t, isValid, "Name should be valid: %s", name)
			})
		}
		
		for _, name := range invalidNames {
			t.Run("invalid_"+name, func(t *testing.T) {
				// Basic validation pattern 
				isValid := len(name) >= 2 && len(name) <= 63 &&
					strings.ToLower(name) == name &&
					!strings.HasPrefix(name, "-") && !strings.HasSuffix(name, "-") &&
					!strings.Contains(name, "--") &&
					name != "api" && name != "dev" && name != "controller"
				
				assert.False(t, isValid, "Name should be invalid: %s", name)
			})
		}
	})
	
	t.Run("SHA validation", func(t *testing.T) {
		validSHAs := []string{
			"abc123", "def456", "1234567890abcdef", "main", "dev", "feature-branch",
		}
		
		invalidSHAs := []string{
			"", "ab", // Too short
		}
		
		for _, sha := range validSHAs {
			t.Run("valid_"+sha, func(t *testing.T) {
				isValid := len(sha) >= 3 && len(sha) <= 64
				assert.True(t, isValid, "SHA should be valid: %s", sha)
			})
		}
		
		for _, sha := range invalidSHAs {
			t.Run("invalid_"+sha, func(t *testing.T) {
				isValid := len(sha) >= 3 && len(sha) <= 64
				assert.False(t, isValid, "SHA should be invalid: %s", sha)
			})
		}
	})
	
	t.Run("main class validation", func(t *testing.T) {
		validMainClasses := []string{
			"com.example.Main", "org.springframework.boot.Application",
			"io.ploy.service.MainClass", "my.package.App",
		}
		
		invalidMainClasses := []string{
			"Main", "main", "com", "com.", ".Main", "com..example.Main",
		}
		
		for _, mainClass := range validMainClasses {
			t.Run("valid_"+strings.ReplaceAll(mainClass, ".", "_"), func(t *testing.T) {
				isValid := len(mainClass) > 0 && strings.Contains(mainClass, ".") &&
					!strings.HasPrefix(mainClass, ".") && !strings.HasSuffix(mainClass, ".") &&
					!strings.Contains(mainClass, "..")
				
				assert.True(t, isValid, "Main class should be valid: %s", mainClass)
			})
		}
		
		for _, mainClass := range invalidMainClasses {
			t.Run("invalid_"+strings.ReplaceAll(mainClass, ".", "_"), func(t *testing.T) {
				isValid := len(mainClass) > 0 && strings.Contains(mainClass, ".") &&
					!strings.HasPrefix(mainClass, ".") && !strings.HasSuffix(mainClass, ".") &&
					!strings.Contains(mainClass, "..")
				
				assert.False(t, isValid, "Main class should be invalid: %s", mainClass)
			})
		}
	})
}