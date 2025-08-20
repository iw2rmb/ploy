package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// StorageClient provides comprehensive error handling, retry logic, and monitoring
type StorageClient struct {
	client      StorageProvider
	retryClient *RetryableStorageClient
	metrics     *StorageMetrics
	healthChecker *HealthChecker
	config      *ClientConfig
}

// ClientConfig configures the storage client
type ClientConfig struct {
	RetryConfig      *RetryConfig      `json:"retry_config"`
	HealthCheckConfig *HealthCheckConfig `json:"health_check_config"`
	EnableMetrics    bool              `json:"enable_metrics"`
	EnableHealthCheck bool             `json:"enable_health_check"`
	MaxOperationTime time.Duration     `json:"max_operation_time"`
}

// DefaultClientConfig returns sensible defaults for storage client
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		RetryConfig:       DefaultRetryConfig(),
		HealthCheckConfig: DefaultHealthCheckConfig(),
		EnableMetrics:     true,
		EnableHealthCheck: true,
		MaxOperationTime:  5 * time.Minute,
	}
}

// NewStorageClient creates a new storage client with comprehensive error handling
func NewStorageClient(client StorageProvider, config *ClientConfig) *StorageClient {
	if config == nil {
		config = DefaultClientConfig()
	}
	
	enhanced := &StorageClient{
		client: client,
		config: config,
	}
	
	// Initialize retry client
	enhanced.retryClient = NewRetryableStorageClient(client, config.RetryConfig)
	
	// Initialize metrics if enabled
	if config.EnableMetrics {
		enhanced.metrics = NewStorageMetrics()
	}
	
	// Initialize health checker if enabled
	if config.EnableHealthCheck && enhanced.metrics != nil {
		enhanced.healthChecker = NewHealthChecker(client, enhanced.metrics, config.HealthCheckConfig)
	}
	
	return enhanced
}

// PutObject uploads an object with comprehensive error handling
func (e *StorageClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	start := time.Now()
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()
	
	// Wrap body with resetter if needed
	bodyWrapper := &fileReadSeekerResetter{readSeeker: body}
	
	// Track file size if possible
	var fileSize int64
	if seeker, ok := body.(*os.File); ok {
		if stat, err := seeker.Stat(); err == nil {
			fileSize = stat.Size()
		}
	}
	
	// Perform the operation with retry logic
	var result *PutObjectResult
	var lastErr error
	
	operation := func() error {
		var err error
		result, err = e.retryClient.PutObject(bucket, key, bodyWrapper, contentType)
		lastErr = err
		return err
	}
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("put_object(%s/%s)", bucket, key))
	
	// Record metrics
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

// GetObject retrieves an object with comprehensive error handling
func (e *StorageClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	start := time.Now()
	
	// Create context with timeout
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
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("get_object(%s/%s)", bucket, key))
	
	// Wrap result with metrics tracking if successful
	if err == nil && result != nil {
		result = &metricsTrackingReadCloser{
			readCloser: result,
			metrics:    e.metrics,
			startTime:  start,
			bytesRead:  &downloadedBytes,
		}
	}
	
	// Record initial metrics (final metrics recorded when reader is closed)
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

// UploadArtifactBundle uploads an artifact bundle with comprehensive error handling
func (e *StorageClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	start := time.Now()
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()
	
	var lastErr error
	
	operation := func() error {
		err := e.retryClient.UploadArtifactBundle(keyPrefix, artifactPath)
		lastErr = err
		return err
	}
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("upload_artifact_bundle(%s)", keyPrefix))
	
	// Record metrics
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

// UploadArtifactBundleWithVerification uploads and verifies with comprehensive error handling
func (e *StorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	start := time.Now()
	
	// Create context with timeout (verification may take longer)
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
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("upload_artifact_bundle_with_verification(%s)", keyPrefix))
	
	// Record metrics for both upload and verification
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

// VerifyUpload verifies an upload with comprehensive error handling
func (e *StorageClient) VerifyUpload(key string) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()
	
	var lastErr error
	
	operation := func() error {
		err := e.retryClient.VerifyUpload(key)
		lastErr = err
		return err
	}
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("verify_upload(%s)", key))
	
	// Record metrics
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

// ListObjects lists objects with comprehensive error handling
func (e *StorageClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxOperationTime)
	defer cancel()
	
	var result []ObjectInfo
	
	operation := func() error {
		var err error
		result, err = e.retryClient.ListObjects(bucket, prefix)
		return err
	}
	
	err := RetryWithBackoff(ctx, operation, e.config.RetryConfig, 
		fmt.Sprintf("list_objects(%s/%s)", bucket, prefix))
	
	if err != nil {
		return nil, fmt.Errorf("object listing failed: %w", err)
	}
	
	return result, nil
}

// GetProviderType returns the storage provider type
func (e *StorageClient) GetProviderType() string {
	return e.client.GetProviderType()
}

// GetArtifactsBucket returns the artifacts bucket name
func (e *StorageClient) GetArtifactsBucket() string {
	return e.client.GetArtifactsBucket()
}

// GetMetrics returns current storage metrics
func (e *StorageClient) GetMetrics() *StorageMetrics {
	if e.metrics == nil {
		return nil
	}
	return e.metrics.GetSnapshot()
}

// GetHealthStatus returns current storage health status
func (e *StorageClient) GetHealthStatus() *HealthCheckResult {
	if e.healthChecker == nil {
		return &HealthCheckResult{
			Status:    HealthStatusUnknown,
			Timestamp: time.Now(),
			Summary:   "Health checking disabled",
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	return e.healthChecker.PerformHealthCheck(ctx)
}

// GetMetricsJSON returns metrics as JSON string
func (e *StorageClient) GetMetricsJSON() ([]byte, error) {
	if e.metrics == nil {
		return []byte("{}"), nil
	}
	return e.metrics.ToJSON()
}

// fileReadSeekerResetter wraps a ReadSeeker to implement Reset functionality
type fileReadSeekerResetter struct {
	readSeeker io.ReadSeeker
}

func (f *fileReadSeekerResetter) Read(p []byte) (int, error) {
	return f.readSeeker.Read(p)
}

func (f *fileReadSeekerResetter) Seek(offset int64, whence int) (int64, error) {
	return f.readSeeker.Seek(offset, whence)
}

func (f *fileReadSeekerResetter) Reset() error {
	_, err := f.readSeeker.Seek(0, 0)
	return err
}

// metricsTrackingReadCloser tracks download metrics
type metricsTrackingReadCloser struct {
	readCloser io.ReadCloser
	metrics    *StorageMetrics
	startTime  time.Time
	bytesRead  *int64
}

func (m *metricsTrackingReadCloser) Read(p []byte) (int, error) {
	n, err := m.readCloser.Read(p)
	if n > 0 {
		*m.bytesRead += int64(n)
	}
	return n, err
}

func (m *metricsTrackingReadCloser) Close() error {
	defer func() {
		if m.metrics != nil {
			duration := time.Since(m.startTime)
			m.metrics.RecordDownload(true, duration, *m.bytesRead, "")
		}
	}()
	
	return m.readCloser.Close()
}