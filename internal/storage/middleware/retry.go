package middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// RetryMiddleware implements storage.Storage with retry logic
type RetryMiddleware struct {
	next   storage.Storage
	config *RetryConfig
}

// RetryConfig defines retry behavior for the middleware
type RetryConfig struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	ShouldRetry       func(err *storage.StorageError, attempt int) bool
}

// DefaultRetryConfig returns sensible defaults for retry middleware
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		ShouldRetry:       defaultShouldRetry,
	}
}

// defaultShouldRetry determines if an error should be retried
func defaultShouldRetry(err *storage.StorageError, attempt int) bool {
	if attempt >= 3 { // Max attempts check
		return false
	}

	// Check if error is retryable
	return err.Retryable
}

// NewRetryMiddleware creates a new retry middleware
func NewRetryMiddleware(next storage.Storage, config *RetryConfig) *RetryMiddleware {
	if config == nil {
		config = DefaultRetryConfig()
	}
	if config.ShouldRetry == nil {
		config.ShouldRetry = defaultShouldRetry
	}

	return &RetryMiddleware{
		next:   next,
		config: config,
	}
}

// calculateBackoff calculates the delay for a retry attempt
func (r *RetryMiddleware) calculateBackoff(attempt int) time.Duration {
	if attempt == 0 {
		return r.config.InitialDelay
	}

	delay := time.Duration(float64(r.config.InitialDelay) *
		(r.config.BackoffMultiplier * float64(attempt)))

	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}

	return delay
}

// retry executes an operation with retry logic
func (r *RetryMiddleware) retry(ctx context.Context, operation func() error, operationName string) error {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is a StorageError and if we should retry
		var storageErr *storage.StorageError
		if se, ok := err.(*storage.StorageError); ok {
			storageErr = se
		} else {
			// Not a storage error, don't retry
			return err
		}

		// Check if we should retry
		if !r.config.ShouldRetry(storageErr, attempt) {
			return storageErr
		}

		// Don't sleep on the last attempt
		if attempt < r.config.MaxAttempts-1 {
			delay := r.calculateBackoff(attempt)

			// Wait before retry, respecting context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	// All attempts exhausted
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Get retrieves an object with retry logic
func (r *RetryMiddleware) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	var reader io.ReadCloser

	err := r.retry(ctx, func() error {
		var err error
		reader, err = r.next.Get(ctx, key)
		return err
	}, "Get")

	return reader, err
}

// Put stores an object with retry logic
func (r *RetryMiddleware) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	return r.retry(ctx, func() error {
		return r.next.Put(ctx, key, reader, opts...)
	}, "Put")
}

// Delete removes an object with retry logic
func (r *RetryMiddleware) Delete(ctx context.Context, key string) error {
	return r.retry(ctx, func() error {
		return r.next.Delete(ctx, key)
	}, "Delete")
}

// Exists checks if an object exists with retry logic
func (r *RetryMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool

	err := r.retry(ctx, func() error {
		var err error
		exists, err = r.next.Exists(ctx, key)
		return err
	}, "Exists")

	return exists, err
}

// List lists objects with retry logic
func (r *RetryMiddleware) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	var objects []storage.Object

	err := r.retry(ctx, func() error {
		var err error
		objects, err = r.next.List(ctx, opts)
		return err
	}, "List")

	return objects, err
}

// DeleteBatch deletes multiple objects with retry logic
func (r *RetryMiddleware) DeleteBatch(ctx context.Context, keys []string) error {
	return r.retry(ctx, func() error {
		return r.next.DeleteBatch(ctx, keys)
	}, "DeleteBatch")
}

// Head gets object metadata with retry logic
func (r *RetryMiddleware) Head(ctx context.Context, key string) (*storage.Object, error) {
	var obj *storage.Object

	err := r.retry(ctx, func() error {
		var err error
		obj, err = r.next.Head(ctx, key)
		return err
	}, "Head")

	return obj, err
}

// UpdateMetadata updates object metadata with retry logic
func (r *RetryMiddleware) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return r.retry(ctx, func() error {
		return r.next.UpdateMetadata(ctx, key, metadata)
	}, "UpdateMetadata")
}

// Copy copies an object with retry logic
func (r *RetryMiddleware) Copy(ctx context.Context, src, dst string) error {
	return r.retry(ctx, func() error {
		return r.next.Copy(ctx, src, dst)
	}, "Copy")
}

// Move moves an object with retry logic
func (r *RetryMiddleware) Move(ctx context.Context, src, dst string) error {
	return r.retry(ctx, func() error {
		return r.next.Move(ctx, src, dst)
	}, "Move")
}

// Health checks storage health with retry logic
func (r *RetryMiddleware) Health(ctx context.Context) error {
	return r.retry(ctx, func() error {
		return r.next.Health(ctx)
	}, "Health")
}

// Metrics returns storage metrics (no retry needed)
func (r *RetryMiddleware) Metrics() *storage.StorageMetrics {
	return r.next.Metrics()
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a StorageError
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		return storageErr.Retryable
	}

	// Default to not retryable for non-storage errors
	return false
}
