package helpers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/testing/helpers"
)

func TestTestRetryConfig(t *testing.T) {
	t.Run("returns valid retry configuration", func(t *testing.T) {
		config := helpers.TestRetryConfig()
		
		require.NotNil(t, config)
		
		// Verify settings are appropriate for unit tests
		assert.Equal(t, 3, config.MaxAttempts, "Should have reasonable max attempts")
		assert.Equal(t, 1*time.Millisecond, config.InitialDelay, "Should have minimal initial delay for fast tests")
		assert.Equal(t, 5*time.Millisecond, config.MaxDelay, "Should have short max delay for fast tests")
		assert.Equal(t, 2.0, config.BackoffMultiplier, "Should have standard backoff multiplier")
	})
	
	t.Run("includes common retryable errors", func(t *testing.T) {
		config := helpers.TestRetryConfig()
		
		require.NotNil(t, config.RetryableErrors)
		assert.Contains(t, config.RetryableErrors, storage.ErrorTypeNetwork, "Should retry network errors")
		assert.Contains(t, config.RetryableErrors, storage.ErrorTypeTimeout, "Should retry timeout errors")
		assert.Contains(t, config.RetryableErrors, storage.ErrorTypeServiceUnavailable, "Should retry service unavailable errors")
		assert.Contains(t, config.RetryableErrors, storage.ErrorTypeRateLimit, "Should retry rate limit errors")
	})
	
	t.Run("delays are suitable for unit tests", func(t *testing.T) {
		config := helpers.TestRetryConfig()
		
		// Calculate total max delay for worst case (all retries with backoff)
		totalDelay := time.Duration(0)
		currentDelay := config.InitialDelay
		
		for i := 0; i < config.MaxAttempts-1; i++ {
			totalDelay += currentDelay
			currentDelay = time.Duration(float64(currentDelay) * config.BackoffMultiplier)
			if currentDelay > config.MaxDelay {
				currentDelay = config.MaxDelay
			}
		}
		
		// Total delay should be less than 100ms for fast unit tests
		assert.Less(t, totalDelay, 100*time.Millisecond, "Total retry delay should be minimal for unit tests")
	})
	
	t.Run("config is immutable between calls", func(t *testing.T) {
		config1 := helpers.TestRetryConfig()
		config2 := helpers.TestRetryConfig()
		
		// Each call should return a new instance
		assert.NotSame(t, config1, config2, "Should return new instance each time")
		
		// But values should be the same
		assert.Equal(t, config1.MaxAttempts, config2.MaxAttempts)
		assert.Equal(t, config1.InitialDelay, config2.InitialDelay)
		assert.Equal(t, config1.MaxDelay, config2.MaxDelay)
		assert.Equal(t, config1.BackoffMultiplier, config2.BackoffMultiplier)
	})
}