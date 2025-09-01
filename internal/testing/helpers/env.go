package helpers

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetEnvOrDefault returns environment variable value or default if not set
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// WithEnvVar temporarily sets an environment variable for the duration of a test
func WithEnvVar(t *testing.T, key, value string) {
	t.Helper()

	oldValue, existed := os.LookupEnv(key)

	err := os.Setenv(key, value)
	require.NoError(t, err)

	t.Cleanup(func() {
		if existed {
			os.Setenv(key, oldValue)
		} else {
			os.Unsetenv(key)
		}
	})
}

// WithEnvVars temporarily sets multiple environment variables
func WithEnvVars(t *testing.T, envVars map[string]string) {
	t.Helper()

	for key, value := range envVars {
		WithEnvVar(t, key, value)
	}
}
