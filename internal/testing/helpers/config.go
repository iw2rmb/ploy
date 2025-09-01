package helpers

import (
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// TestRetryConfig returns a retry configuration suitable for unit tests
// with minimal delays to keep tests fast
func TestRetryConfig() *storage.RetryConfig {
	return &storage.RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Millisecond, // Minimal delay for unit tests
		MaxDelay:          5 * time.Millisecond, // Keep very short
		BackoffMultiplier: 2.0,
		RetryableErrors: []storage.ErrorType{
			storage.ErrorTypeNetwork,
			storage.ErrorTypeTimeout,
			storage.ErrorTypeServiceUnavailable,
			storage.ErrorTypeRateLimit,
		},
	}
}
