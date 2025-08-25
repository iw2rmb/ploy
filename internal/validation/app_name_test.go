package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAppName(t *testing.T) {
	tests := []struct {
		name          string
		appName       string
		expectedError string
	}{
		// Valid app names
		{
			name:    "valid simple name",
			appName: "myapp",
		},
		{
			name:    "valid name with numbers",
			appName: "app123",
		},
		{
			name:    "valid name with hyphens",
			appName: "my-app",
		},
		{
			name:    "valid complex name",
			appName: "my-java-app-v2",
		},
		{
			name:    "valid name ending with number",
			appName: "test123",
		},
		{
			name:    "valid minimum length",
			appName: "ab",
		},
		{
			name:    "valid maximum length (63 chars)",
			appName: strings.Repeat("a", 61) + "bc", // 63 chars total, ends with letter
		},
		{
			name:    "valid uppercase converted to lowercase",
			appName: "MyApp",
		},

		// Invalid app names - empty and length
		{
			name:          "empty name",
			appName:       "",
			expectedError: "app name cannot be empty",
		},
		{
			name:          "too short",
			appName:       "a",
			expectedError: "app name must be at least 2 characters long",
		},
		{
			name:          "too long",
			appName:       strings.Repeat("a", 64),
			expectedError: "app name cannot exceed 63 characters",
		},

		// Invalid app names - reserved names
		{
			name:          "reserved api",
			appName:       "api",
			expectedError: "app name 'api' is reserved for platform use",
		},
		{
			name:          "reserved dev",
			appName:       "dev",
			expectedError: "app name 'dev' is reserved for platform use",
		},
		{
			name:          "reserved controller",
			appName:       "controller",
			expectedError: "app name 'controller' is reserved for platform use",
		},
		{
			name:          "reserved admin",
			appName:       "admin",
			expectedError: "app name 'admin' is reserved for platform use",
		},
		{
			name:          "reserved dashboard",
			appName:       "dashboard",
			expectedError: "app name 'dashboard' is reserved for platform use",
		},
		{
			name:          "reserved metrics",
			appName:       "metrics",
			expectedError: "app name 'metrics' is reserved for platform use",
		},
		{
			name:          "reserved health",
			appName:       "health",
			expectedError: "app name 'health' is reserved for platform use",
		},
		{
			name:          "reserved console",
			appName:       "console",
			expectedError: "app name 'console' is reserved for platform use",
		},
		{
			name:          "reserved www",
			appName:       "www",
			expectedError: "app name 'www' is reserved for platform use",
		},
		{
			name:          "reserved ploy",
			appName:       "ploy",
			expectedError: "app name 'ploy' is reserved for platform use",
		},
		{
			name:          "reserved system",
			appName:       "system",
			expectedError: "app name 'system' is reserved for platform use",
		},
		{
			name:          "reserved traefik",
			appName:       "traefik",
			expectedError: "app name 'traefik' is reserved for platform use",
		},
		{
			name:          "reserved nomad",
			appName:       "nomad",
			expectedError: "app name 'nomad' is reserved for platform use",
		},
		{
			name:          "reserved consul",
			appName:       "consul",
			expectedError: "app name 'consul' is reserved for platform use",
		},
		{
			name:          "reserved vault",
			appName:       "vault",
			expectedError: "app name 'vault' is reserved for platform use",
		},
		{
			name:          "reserved seaweedfs",
			appName:       "seaweedfs",
			expectedError: "app name 'seaweedfs' is reserved for platform use",
		},
		{
			name:          "reserved name case insensitive",
			appName:       "API",
			expectedError: "app name 'api' is reserved for platform use",
		},

		// Invalid app names - pattern violations
		{
			name:          "starts with number",
			appName:       "123app",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "starts with hyphen",
			appName:       "-app",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "ends with hyphen",
			appName:       "app-",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "contains uppercase after conversion",
			appName:       "app_name", // underscore not allowed
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "contains special characters",
			appName:       "app@name",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "contains spaces",
			appName:       "my app",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},
		{
			name:          "contains dots",
			appName:       "app.name",
			expectedError: "app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens",
		},

		// Invalid app names - consecutive hyphens
		{
			name:          "double hyphens",
			appName:       "app--name",
			expectedError: "app name cannot contain consecutive hyphens",
		},
		{
			name:          "triple hyphens",
			appName:       "app---name",
			expectedError: "app name cannot contain consecutive hyphens",
		},
		{
			name:          "multiple double hyphens",
			appName:       "app--name--test",
			expectedError: "app name cannot contain consecutive hyphens",
		},

		// Invalid app names - reserved prefixes
		{
			name:          "ploy prefix",
			appName:       "ploy-test",
			expectedError: "app name cannot start with reserved prefixes: ploy-, system-",
		},
		{
			name:          "system prefix",
			appName:       "system-app",
			expectedError: "app name cannot start with reserved prefixes: ploy-, system-",
		},
		{
			name:          "ploy prefix case insensitive",
			appName:       "PLOY-test",
			expectedError: "app name cannot start with reserved prefixes: ploy-, system-",
		},
		{
			name:          "system prefix case insensitive",
			appName:       "System-App",
			expectedError: "app name cannot start with reserved prefixes: ploy-, system-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAppName(tt.appName)
			if tt.expectedError == "" {
				assert.NoError(t, err, "expected valid app name")
			} else {
				require.Error(t, err, "expected validation error")
				assert.Contains(t, err.Error(), tt.expectedError, "error message should match")
			}
		})
	}
}

func TestValidateAppName_EdgeCases(t *testing.T) {
	t.Run("boundary length test - exactly 63 characters", func(t *testing.T) {
		// Create exactly 63 character name: start with 'a', fill middle with 'b', end with 'c'
		appName := "a" + strings.Repeat("b", 61) + "c"
		assert.Len(t, appName, 63)
		
		err := ValidateAppName(appName)
		assert.NoError(t, err)
	})

	t.Run("boundary length test - exactly 64 characters", func(t *testing.T) {
		// Create exactly 64 character name
		appName := strings.Repeat("a", 64)
		assert.Len(t, appName, 64)
		
		err := ValidateAppName(appName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot exceed 63 characters")
	})

	t.Run("mixed case conversion", func(t *testing.T) {
		testCases := []string{
			"MyApp",
			"MY-APP",
			"mY-aPp-TeSt",
			"CamelCaseApp",
		}
		
		for _, testCase := range testCases {
			err := ValidateAppName(testCase)
			assert.NoError(t, err, "mixed case should be converted to lowercase and be valid")
		}
	})

	t.Run("unicode characters", func(t *testing.T) {
		testCases := []string{
			"appñame",     // Spanish character
			"app名前",      // Japanese characters
			"appémilie",   // French characters
			"приложение",   // Cyrillic characters
		}
		
		for _, testCase := range testCases {
			err := ValidateAppName(testCase)
			assert.Error(t, err, "unicode characters should not be allowed")
			assert.Contains(t, err.Error(), "must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens")
		}
	})
}

func TestIsReservedAppName(t *testing.T) {
	tests := []struct {
		name     string
		appName  string
		expected bool
	}{
		// Reserved names
		{
			name:     "api is reserved",
			appName:  "api",
			expected: true,
		},
		{
			name:     "dev is reserved",
			appName:  "dev",
			expected: true,
		},
		{
			name:     "controller is reserved",
			appName:  "controller",
			expected: true,
		},
		{
			name:     "ploy is reserved",
			appName:  "ploy",
			expected: true,
		},
		{
			name:     "nomad is reserved",
			appName:  "nomad",
			expected: true,
		},

		// Case insensitive reserved names
		{
			name:     "API is reserved (case insensitive)",
			appName:  "API",
			expected: true,
		},
		{
			name:     "Controller is reserved (case insensitive)",
			appName:  "Controller",
			expected: true,
		},
		{
			name:     "PLOY is reserved (case insensitive)",
			appName:  "PLOY",
			expected: true,
		},

		// Non-reserved names
		{
			name:     "myapp is not reserved",
			appName:  "myapp",
			expected: false,
		},
		{
			name:     "test-app is not reserved",
			appName:  "test-app",
			expected: false,
		},
		{
			name:     "hello-world is not reserved",
			appName:  "hello-world",
			expected: false,
		},
		{
			name:     "api-client is not reserved (contains but doesn't equal)",
			appName:  "api-client",
			expected: false,
		},
		{
			name:     "dev-tools is not reserved (contains but doesn't equal)",
			appName:  "dev-tools",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty string is not reserved",
			appName:  "",
			expected: false,
		},
		{
			name:     "single character is not reserved",
			appName:  "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsReservedAppName(tt.appName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetReservedAppNames(t *testing.T) {
	t.Run("returns all reserved names", func(t *testing.T) {
		names := GetReservedAppNames()
		
		// Check that we have the expected count
		expectedCount := len(reservedAppNames)
		assert.Len(t, names, expectedCount, "should return all reserved names")
		
		// Check that all expected names are present
		expectedNames := []string{
			"api", "dev", "controller", "admin", "dashboard", 
			"metrics", "health", "console", "www", "ploy", 
			"system", "traefik", "nomad", "consul", "vault", "seaweedfs",
		}
		
		for _, expected := range expectedNames {
			assert.Contains(t, names, expected, "should contain reserved name: %s", expected)
		}
		
		// Verify no duplicates
		nameMap := make(map[string]bool)
		for _, name := range names {
			assert.False(t, nameMap[name], "should not contain duplicates: %s", name)
			nameMap[name] = true
		}
	})

	t.Run("returned names are all reserved", func(t *testing.T) {
		names := GetReservedAppNames()
		
		for _, name := range names {
			assert.True(t, IsReservedAppName(name), "returned name should be reserved: %s", name)
		}
	})

	t.Run("consistency check", func(t *testing.T) {
		// Ensure the slice matches the map
		names := GetReservedAppNames()
		
		assert.Len(t, names, len(reservedAppNames), "slice length should match map length")
		
		// Every name in slice should be in map
		for _, name := range names {
			assert.True(t, reservedAppNames[name], "name from slice should exist in map: %s", name)
		}
		
		// Every name in map should be in slice
		nameSliceMap := make(map[string]bool)
		for _, name := range names {
			nameSliceMap[name] = true
		}
		
		for mapName := range reservedAppNames {
			assert.True(t, nameSliceMap[mapName], "name from map should exist in slice: %s", mapName)
		}
	})
}

func TestAppNamePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid patterns
		{
			name:     "simple lowercase",
			input:    "app",
			expected: true,
		},
		{
			name:     "with numbers",
			input:    "app123",
			expected: true,
		},
		{
			name:     "with hyphens",
			input:    "my-app",
			expected: true,
		},
		{
			name:     "complex valid",
			input:    "my-app-v2-test",
			expected: true,
		},
		{
			name:     "minimum valid (2 chars)",
			input:    "ab",
			expected: true,
		},
		{
			name:     "ends with number",
			input:    "test123",
			expected: true,
		},

		// Invalid patterns
		{
			name:     "starts with number",
			input:    "123app",
			expected: false,
		},
		{
			name:     "starts with hyphen",
			input:    "-app",
			expected: false,
		},
		{
			name:     "ends with hyphen",
			input:    "app-",
			expected: false,
		},
		{
			name:     "contains uppercase",
			input:    "App",
			expected: false,
		},
		{
			name:     "contains underscore",
			input:    "my_app",
			expected: false,
		},
		{
			name:     "contains space",
			input:    "my app",
			expected: false,
		},
		{
			name:     "contains special characters",
			input:    "app@test",
			expected: false,
		},
		{
			name:     "single character",
			input:    "a",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "too long (over 63 chars)",
			input:    strings.Repeat("a", 64),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppNamePattern.MatchString(tt.input)
			assert.Equal(t, tt.expected, result, "pattern match should be %t for input: %s", tt.expected, tt.input)
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkValidateAppName(b *testing.B) {
	testNames := []string{
		"myapp",
		"my-complex-app-name-v123",
		"api", // reserved
		"invalid--name",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			ValidateAppName(name)
		}
	}
}

func BenchmarkIsReservedAppName(b *testing.B) {
	testNames := []string{
		"api",
		"myapp", 
		"controller",
		"test-app",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			IsReservedAppName(name)
		}
	}
}

func BenchmarkAppNamePattern(b *testing.B) {
	testNames := []string{
		"myapp",
		"my-app-123",
		"123invalid",
		"app-",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			AppNamePattern.MatchString(name)
		}
	}
}

// Property-based testing helpers
func TestValidateAppName_Properties(t *testing.T) {
	t.Run("idempotency - validating twice gives same result", func(t *testing.T) {
		testCases := []string{
			"myapp",
			"api", // reserved
			"invalid--name",
			"123invalid",
		}
		
		for _, testCase := range testCases {
			err1 := ValidateAppName(testCase)
			err2 := ValidateAppName(testCase)
			
			if err1 == nil {
				assert.NoError(t, err2, "second validation should also succeed")
			} else {
				require.Error(t, err2, "second validation should also fail")
				assert.Equal(t, err1.Error(), err2.Error(), "error messages should be identical")
			}
		}
	})

	t.Run("case insensitivity - uppercase/lowercase give same result", func(t *testing.T) {
		testPairs := []struct {
			lower string
			upper string
		}{
			{"myapp", "MYAPP"},
			{"my-app", "MY-APP"}, 
			{"test123", "TEST123"},
			{"api", "API"}, // reserved
		}
		
		for _, pair := range testPairs {
			lowerErr := ValidateAppName(pair.lower)
			upperErr := ValidateAppName(pair.upper)
			
			if lowerErr == nil {
				assert.NoError(t, upperErr, "uppercase version should also be valid")
			} else {
				require.Error(t, upperErr, "uppercase version should also be invalid")
				// Note: error messages might differ due to case conversion in error text
			}
		}
	})
}