package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryWithBackoff(t *testing.T) {
	tests := []struct {
		name           string
		operation      func() RetryOperation
		config         *RetryConfig
		operationName  string
		expectError    bool
		expectAttempts int
	}{
		{
			name: "success on first attempt",
			operation: func() RetryOperation {
				return func() error {
					return nil
				}
			},
			config:         DefaultRetryConfig(),
			operationName:  "test_success",
			expectError:    false,
			expectAttempts: 1,
		},
		{
			name: "success on second attempt",
			operation: func() RetryOperation {
				attempt := 0
				return func() error {
					attempt++
					if attempt == 1 {
						return NewStorageError("test", errors.New("network error"), ErrorContext{})
					}
					return nil
				}
			},
			config:         DefaultRetryConfig(),
			operationName:  "test_retry_success",
			expectError:    false,
			expectAttempts: 2,
		},
		{
			name: "non-retryable error",
			operation: func() RetryOperation {
				return func() error {
					return &StorageError{
						ErrorType: ErrorTypeAuthentication,
						Retryable: false,
						Message:   "auth failed",
					}
				}
			},
			config:         DefaultRetryConfig(),
			operationName:  "test_non_retryable",
			expectError:    true,
			expectAttempts: 1,
		},
		{
			name: "all attempts exhausted",
			operation: func() RetryOperation {
				return func() error {
					return NewStorageError("test", errors.New("persistent error"), ErrorContext{})
				}
			},
			config: &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors:   []ErrorType{ErrorTypeInternal},
			},
			operationName:  "test_exhausted",
			expectError:    true,
			expectAttempts: 2,
		},
		{
			name: "context cancellation",
			operation: func() RetryOperation {
				return func() error {
					return NewStorageError("test", errors.New("network error"), ErrorContext{})
				}
			},
			config:         DefaultRetryConfig(),
			operationName:  "test_cancelled",
			expectError:    true,
			expectAttempts: 1, // Cancelled during retry delay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.name == "context cancellation" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				// Cancel after a short delay to simulate cancellation during retry
				go func() {
					time.Sleep(5 * time.Millisecond)
					cancel()
				}()
			}

			operation := tt.operation()
			err := RetryWithBackoff(ctx, operation, tt.config, tt.operationName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRetryWithBackoff_RespectsRetryAfter(t *testing.T) {
	// Configure a large initial delay so we can detect Retry-After override
	cfg := &RetryConfig{MaxAttempts: 2, InitialDelay: 200 * time.Millisecond, MaxDelay: 1 * time.Second, BackoffMultiplier: 2.0, RetryableErrors: []ErrorType{ErrorTypeInternal}}
	attempts := 0
	op := func() error {
		attempts++
		if attempts == 1 {
			// Return a retryable StorageError with RetryAfter 5ms
			return &StorageError{ErrorType: ErrorTypeInternal, Retryable: true, RetryAfter: 5 * time.Millisecond}
		}
		return nil
	}
	start := time.Now()
	err := RetryWithBackoff(context.Background(), op, cfg, "test_retry_after")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	// Should be significantly less than InitialDelay due to Retry-After override
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("expected elapsed < 200ms due to Retry-After override, got %v", elapsed)
	}
}

// Benchmark tests for retry performance
func BenchmarkRetryWithBackoff_Success(b *testing.B) {
	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Microsecond,
		MaxDelay:          10 * time.Microsecond,
		BackoffMultiplier: 2.0,
	}

	operation := func() error {
		return nil // Always succeed
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RetryWithBackoff(context.Background(), operation, config, "benchmark")
	}
}

func BenchmarkRetryWithBackoff_OneRetry(b *testing.B) {
	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Microsecond,
		MaxDelay:          10 * time.Microsecond,
		BackoffMultiplier: 2.0,
		RetryableErrors:   []ErrorType{ErrorTypeNetwork},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempt := 0
		operation := func() error {
			attempt++
			if attempt == 1 {
				return &StorageError{
					ErrorType: ErrorTypeNetwork,
					Retryable: true,
				}
			}
			return nil
		}
		_ = RetryWithBackoff(context.Background(), operation, config, "benchmark")
	}
}
