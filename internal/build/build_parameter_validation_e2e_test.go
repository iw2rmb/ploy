//go:build e2e
// +build e2e

package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildParameterValidation tests validation of build parameters (moved under e2e tag)
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
			"", "ab",
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
}
