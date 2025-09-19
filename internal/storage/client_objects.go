package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// PutObject uploads an object with comprehensive error handling.
func (e *StorageClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()

	bodyWrapper := &fileReadSeekerResetter{readSeeker: body}

	var fileSize int64
	if seeker, ok := body.(*os.File); ok {
		if stat, err := seeker.Stat(); err == nil {
			fileSize = stat.Size()
		}
	}

	var result *PutObjectResult
	var lastErr error

	operation := func() error {
		var err error
		result, err = e.retryClient.PutObject(bucket, key, bodyWrapper, contentType)
		lastErr = err
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("put_object(%s/%s)", bucket, key))

	if e.metrics != nil {
		duration := time.Since(start)
		success := err == nil
		var errorType ErrorType
		if !success && lastErr != nil {
			if storageErr, ok := lastErr.(*StorageError); ok {
				errorType = storageErr.ErrorType
			}
		}
		e.metrics.RecordUpload(success, duration, fileSize, errorType)
	}

	if err != nil {
		return nil, fmt.Errorf("storage put operation failed: %w", err)
	}

	return result, nil
}

// GetObject retrieves an object with comprehensive error handling.
func (e *StorageClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()

	var result io.ReadCloser
	var lastErr error
	var downloadedBytes int64

	operation := func() error {
		var err error
		result, err = e.retryClient.GetObject(bucket, key)
		lastErr = err
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("get_object(%s/%s)", bucket, key))

	if err == nil && result != nil {
		result = &metricsTrackingReadCloser{readCloser: result, metrics: e.metrics, startTime: start, bytesRead: &downloadedBytes}
	}

	if e.metrics != nil && err != nil {
		duration := time.Since(start)
		var errorType ErrorType
		if lastErr != nil {
			if storageErr, ok := lastErr.(*StorageError); ok {
				errorType = storageErr.ErrorType
			}
		}
		e.metrics.RecordDownload(false, duration, 0, errorType)
	}

	if err != nil {
		return nil, fmt.Errorf("storage get operation failed: %w", err)
	}

	return result, nil
}

// UploadArtifactBundle uploads an artifact bundle with comprehensive error handling.
func (e *StorageClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()

	var lastErr error

	operation := func() error {
		err := e.retryClient.UploadArtifactBundle(keyPrefix, artifactPath)
		lastErr = err
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("upload_artifact_bundle(%s)", keyPrefix))

	if e.metrics != nil {
		duration := time.Since(start)
		success := err == nil
		var errorType ErrorType
		if !success && lastErr != nil {
			if storageErr, ok := lastErr.(*StorageError); ok {
				errorType = storageErr.ErrorType
			}
		}
		e.metrics.RecordUpload(success, duration, 0, errorType)
	}

	if err != nil {
		return fmt.Errorf("artifact bundle upload failed: %w", err)
	}

	return nil
}

// UploadArtifactBundleWithVerification uploads and verifies with comprehensive error handling.
func (e *StorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime*2)
	defer cancel()

	var result *BundleIntegrityResult
	var lastErr error

	operation := func() error {
		var err error
		result, err = e.retryClient.UploadArtifactBundleWithVerification(keyPrefix, artifactPath)
		lastErr = err
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("upload_artifact_bundle_with_verification(%s)", keyPrefix))

	if e.metrics != nil {
		duration := time.Since(start)
		success := err == nil && result != nil && result.Verified
		var errorType ErrorType
		if !success && lastErr != nil {
			if storageErr, ok := lastErr.(*StorageError); ok {
				errorType = storageErr.ErrorType
			}
		}
		e.metrics.RecordUpload(success, duration, 0, errorType)
		e.metrics.RecordVerification(success, errorType)
	}

	if err != nil {
		return nil, fmt.Errorf("artifact bundle upload with verification failed: %w", err)
	}

	return result, nil
}

// VerifyUpload verifies an upload with comprehensive error handling.
func (e *StorageClient) VerifyUpload(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()

	var lastErr error

	operation := func() error {
		err := e.retryClient.VerifyUpload(key)
		lastErr = err
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("verify_upload(%s)", key))

	if e.metrics != nil {
		success := err == nil
		var errorType ErrorType
		if !success && lastErr != nil {
			if storageErr, ok := lastErr.(*StorageError); ok {
				errorType = storageErr.ErrorType
			}
		}
		e.metrics.RecordVerification(success, errorType)
	}

	if err != nil {
		return fmt.Errorf("upload verification failed: %w", err)
	}
	return nil
}

// ListObjects lists objects with comprehensive error handling.
func (e *StorageClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()

	var result []ObjectInfo
	operation := func() error {
		var err error
		result, err = e.retryClient.ListObjects(bucket, prefix)
		return err
	}

	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, fmt.Sprintf("list_objects(%s/%s)", bucket, prefix))
	if err != nil {
		return nil, fmt.Errorf("object listing failed: %w", err)
	}
	return result, nil
}

// Minimal Storage interface implementations.
func (e *StorageClient) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	bucket := e.client.GetArtifactsBucket()
	return e.GetObject(bucket, key)
}

func (e *StorageClient) Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error {
	options := &putOptions{}
	for _, opt := range opts {
		opt(options)
	}

	bucket := e.client.GetArtifactsBucket()
	contentType := options.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	rs, ok := reader.(io.ReadSeeker)
	if !ok {
		return fmt.Errorf("reader conversion not implemented")
	}

	_, err := e.PutObject(bucket, key, rs, contentType)
	return err
}

func (e *StorageClient) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("Delete operation not implemented")
}

func (e *StorageClient) Exists(ctx context.Context, key string) (bool, error) {
	err := e.VerifyUpload(key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (e *StorageClient) List(ctx context.Context, opts ListOptions) ([]Object, error) {
	bucket := e.client.GetArtifactsBucket()
	objectInfos, err := e.ListObjects(bucket, opts.Prefix)
	if err != nil {
		return nil, err
	}

	objects := make([]Object, len(objectInfos))
	for i, info := range objectInfos {
		objects[i] = Object{
			Key:         info.Key,
			Size:        info.Size,
			ContentType: info.ContentType,
			ETag:        info.ETag,
			Metadata:    make(map[string]string),
		}
	}
	return objects, nil
}

func (e *StorageClient) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := e.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (e *StorageClient) Head(ctx context.Context, key string) (*Object, error) {
	return nil, fmt.Errorf("Head operation not implemented")
}

func (e *StorageClient) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return fmt.Errorf("UpdateMetadata operation not implemented")
}

func (e *StorageClient) Copy(ctx context.Context, src, dst string) error {
	return fmt.Errorf("Copy operation not implemented")
}

func (e *StorageClient) Move(ctx context.Context, src, dst string) error {
	return fmt.Errorf("Move operation not implemented")
}
