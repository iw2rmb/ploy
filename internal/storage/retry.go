package storage

import (
	"context"
	"fmt"
	"time"
)

// RetryOperation represents a retryable storage operation
type RetryOperation func() error

// RetryWithBackoff executes an operation with exponential backoff retry logic
func RetryWithBackoff(ctx context.Context, operation RetryOperation, config *RetryConfig, operationName string) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr *StorageError

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			// Success
			if attempt > 0 {
				fmt.Printf("Storage operation '%s' succeeded after %d attempts\n", operationName, attempt+1)
			}
			return nil
		}

		// Convert to StorageError if not already
		var storageErr *StorageError
		if se, ok := err.(*StorageError); ok {
			storageErr = se
		} else {
			storageErr = NewStorageError(operationName, err, ErrorContext{
				AttemptNumber: attempt + 1,
			})
		}

		lastErr = storageErr

		// Check if we should retry
		if !config.ShouldRetry(storageErr, attempt) {
			fmt.Printf("Storage operation '%s' failed (non-retryable): %v\n", operationName, storageErr)
			return storageErr
		}

		// Calculate delay for next attempt
		var delay time.Duration
		if storageErr.RetryAfter > 0 {
			delay = storageErr.RetryAfter
		} else {
			delay = config.CalculateDelay(attempt)
		}

		fmt.Printf("Storage operation '%s' attempt %d failed (%s), retrying in %v: %s\n",
			operationName, attempt+1, storageErr.ErrorType, delay, storageErr.Message)

		// Wait before retry, respecting context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("storage operation '%s' cancelled: %w", operationName, ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// All attempts exhausted
	return fmt.Errorf("storage operation '%s' failed after %d attempts: %w",
		operationName, config.MaxAttempts, lastErr)
}

// RetryableStorageClient wraps a storage client with comprehensive retry logic
type RetryableStorageClient struct {
	client StorageProvider
	config *RetryConfig
}

// NewRetryableStorageClient creates a new storage client with retry capabilities
func NewRetryableStorageClient(client StorageProvider, config *RetryConfig) *RetryableStorageClient {
	if config == nil {
		config = DefaultRetryConfig()
	}

	return &RetryableStorageClient{
		client: client,
		config: config,
	}
}

// PutObject uploads an object with retry logic
func (r *RetryableStorageClient) PutObject(bucket, key string, body ReadSeekerResetter, contentType string) (*PutObjectResult, error) {
	ctx := context.Background()
	var result *PutObjectResult

	operation := func() error {
		// Reset body to beginning before each attempt
		if err := body.Reset(); err != nil {
			return NewStorageError("put_object", err, ErrorContext{
				Bucket:      bucket,
				Key:         key,
				ContentType: contentType,
			})
		}

		var err error
		result, err = r.client.PutObject(bucket, key, body, contentType)
		if err != nil {
			// Enhance error with context
			return NewStorageError("put_object", err, ErrorContext{
				Bucket:      bucket,
				Key:         key,
				ContentType: contentType,
			})
		}
		return nil
	}

	err := RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("put_object(%s/%s)", bucket, key))
	return result, err
}

// GetObject retrieves an object with retry logic
func (r *RetryableStorageClient) GetObject(bucket, key string) (ReadCloserWithRetry, error) {
	ctx := context.Background()
	var reader ReadCloserWithRetry

	operation := func() error {
		rc, err := r.client.GetObject(bucket, key)
		if err != nil {
			return NewStorageError("get_object", err, ErrorContext{
				Bucket: bucket,
				Key:    key,
			})
		}
		reader = &retryableReadCloser{
			reader: rc,
			client: r.client,
			bucket: bucket,
			key:    key,
			config: r.config,
		}
		return nil
	}

	err := RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("get_object(%s/%s)", bucket, key))
	return reader, err
}

// UploadArtifactBundle uploads an artifact bundle with retry logic
func (r *RetryableStorageClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	ctx := context.Background()

	operation := func() error {
		err := r.client.UploadArtifactBundle(keyPrefix, artifactPath)
		if err != nil {
			return NewStorageError("upload_artifact_bundle", err, ErrorContext{
				Key: keyPrefix,
			})
		}
		return nil
	}

	return RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("upload_artifact_bundle(%s)", keyPrefix))
}

// UploadArtifactBundleWithVerification uploads and verifies an artifact bundle with retry logic
func (r *RetryableStorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	ctx := context.Background()
	var result *BundleIntegrityResult

	operation := func() error {
		var err error
		result, err = r.client.UploadArtifactBundleWithVerification(keyPrefix, artifactPath)
		if err != nil {
			return NewStorageError("upload_artifact_bundle_with_verification", err, ErrorContext{
				Key: keyPrefix,
			})
		}

		// Additional validation of verification result
		if result != nil && !result.Verified {
			return NewStorageError("upload_artifact_bundle_with_verification",
				fmt.Errorf("integrity verification failed"), ErrorContext{
					Key: keyPrefix,
				})
		}

		return nil
	}

	err := RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("upload_artifact_bundle_with_verification(%s)", keyPrefix))
	return result, err
}

// VerifyUpload verifies an upload with retry logic
func (r *RetryableStorageClient) VerifyUpload(key string) error {
	ctx := context.Background()

	operation := func() error {
		err := r.client.VerifyUpload(key)
		if err != nil {
			return NewStorageError("verify_upload", err, ErrorContext{
				Key: key,
			})
		}
		return nil
	}

	return RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("verify_upload(%s)", key))
}

// ListObjects lists objects with retry logic
func (r *RetryableStorageClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	ctx := context.Background()
	var objects []ObjectInfo

	operation := func() error {
		var err error
		objects, err = r.client.ListObjects(bucket, prefix)
		if err != nil {
			return NewStorageError("list_objects", err, ErrorContext{
				Bucket: bucket,
				Key:    prefix,
			})
		}
		return nil
	}

	err := RetryWithBackoff(ctx, operation, r.config, fmt.Sprintf("list_objects(%s/%s)", bucket, prefix))
	return objects, err
}

// GetProviderType returns the storage provider type
func (r *RetryableStorageClient) GetProviderType() string {
	return r.client.GetProviderType()
}

// GetArtifactsBucket returns the artifacts bucket name
func (r *RetryableStorageClient) GetArtifactsBucket() string {
	return r.client.GetArtifactsBucket()
}

// ReadSeekerResetter interface for objects that can be reset to the beginning
type ReadSeekerResetter interface {
	ReadSeeker
	Reset() error
}

// ReadCloserWithRetry interface for read closers with retry capabilities
type ReadCloserWithRetry interface {
	ReadCloser
	Retry() error
}

// retryableReadCloser implements retry logic for read operations
type retryableReadCloser struct {
	reader ReadCloser
	client StorageProvider
	bucket string
	key    string
	config *RetryConfig
}

func (r *retryableReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil && isRetryableReadError(err) {
		// Try to recover by reopening the stream
		if retryErr := r.Retry(); retryErr == nil {
			// Retry the read operation
			return r.reader.Read(p)
		}
	}
	return n, err
}

func (r *retryableReadCloser) Close() error {
	if r.reader != nil {
		return r.reader.Close()
	}
	return nil
}

func (r *retryableReadCloser) Retry() error {
	// Close existing reader
	if r.reader != nil {
		r.reader.Close()
	}

	// Reopen the stream
	newReader, err := r.client.GetObject(r.bucket, r.key)
	if err != nil {
		return err
	}

	r.reader = newReader
	return nil
}

// isRetryableReadError determines if a read error should trigger a retry
func isRetryableReadError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network-related read errors
	if isNetworkError(err) || isTimeoutError(err) {
		return true
	}

	return false
}

// ReadSeeker interface (re-exported for convenience)
type ReadSeeker interface {
	Read([]byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
}

// ReadCloser interface (re-exported for convenience)
type ReadCloser interface {
	Read([]byte) (int, error)
	Close() error
}
