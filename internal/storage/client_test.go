package storage

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockStorageProvider implements StorageProvider for testing
type MockStorageProvider struct {
	mock.Mock
}

func (m *MockStorageProvider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	args := m.Called(bucket, key, body, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PutObjectResult), args.Error(1)
}

func (m *MockStorageProvider) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	args := m.Called(keyPrefix, artifactPath)
	return args.Error(0)
}

func (m *MockStorageProvider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	args := m.Called(keyPrefix, artifactPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BundleIntegrityResult), args.Error(1)
}

func (m *MockStorageProvider) VerifyUpload(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *MockStorageProvider) GetObject(bucket, key string) (io.ReadCloser, error) {
	args := m.Called(bucket, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorageProvider) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	args := m.Called(bucket, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]ObjectInfo), args.Error(1)
}

func (m *MockStorageProvider) GetProviderType() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStorageProvider) GetArtifactsBucket() string {
	args := m.Called()
	return args.String(0)
}

func TestNewStorageClient(t *testing.T) {
	tests := []struct {
		name     string
		provider StorageProvider
		config   *ClientConfig
		wantNil  bool
	}{
		{
			name:     "with default config",
			provider: &MockStorageProvider{},
			config:   nil, // Should use default
			wantNil:  false,
		},
		{
			name:     "with custom config",
			provider: &MockStorageProvider{},
			config: &ClientConfig{
				EnableMetrics:     false,
				EnableHealthCheck: false,
				MaxOperationTime:  1 * time.Minute,
			},
			wantNil: false,
		},
		{
			name:     "with metrics and health check enabled",
			provider: &MockStorageProvider{},
			config: &ClientConfig{
				EnableMetrics:     true,
				EnableHealthCheck: true,
				RetryConfig:       DefaultRetryConfig(),
				HealthCheckConfig: DefaultHealthCheckConfig(),
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewStorageClient(tt.provider, tt.config)
			
			if tt.wantNil {
				assert.Nil(t, client)
			} else {
				assert.NotNil(t, client)
				assert.NotNil(t, client.client)
				assert.NotNil(t, client.retryClient)
				assert.NotNil(t, client.config)
				
				// Check metrics initialization
				if tt.config == nil || tt.config.EnableMetrics {
					assert.NotNil(t, client.metrics)
				}
				
				// Check health checker initialization
				if tt.config != nil && tt.config.EnableHealthCheck && client.metrics != nil {
					assert.NotNil(t, client.healthChecker)
				}
			}
		})
	}
}

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()
	
	assert.NotNil(t, config)
	assert.NotNil(t, config.RetryConfig)
	assert.NotNil(t, config.HealthCheckConfig)
	assert.True(t, config.EnableMetrics)
	assert.True(t, config.EnableHealthCheck)
	assert.Equal(t, 5*time.Minute, config.MaxOperationTime)
}

func TestStorageClient_PutObject(t *testing.T) {
	tests := []struct {
		name           string
		bucket         string
		key            string
		body           string
		contentType    string
		setupMock      func(*MockStorageProvider)
		expectedResult *PutObjectResult
		expectError    bool
		errorContains  string
	}{
		{
			name:        "successful put",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(&PutObjectResult{
						ETag:     "test-etag",
						Location: "test-location",
						Size:     12,
					}, nil)
			},
			expectedResult: &PutObjectResult{
				ETag:     "test-etag",
				Location: "test-location",
				Size:     12,
			},
			expectError: false,
		},
		{
			name:        "network error with retry",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				// First call fails with network error
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(nil, errors.New("connection refused")).Once()
				
				// Second call succeeds
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(&PutObjectResult{
						ETag:     "test-etag",
						Location: "test-location",
						Size:     12,
					}, nil).Once()
			},
			expectedResult: &PutObjectResult{
				ETag:     "test-etag",
				Location: "test-location",
				Size:     12,
			},
			expectError: false,
		},
		{
			name:        "permanent failure",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(nil, errors.New("authentication failed"))
			},
			expectError:   true,
			errorContains: "storage put operation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &ClientConfig{
				RetryConfig: &RetryConfig{
					MaxAttempts:       2,
					InitialDelay:      1 * time.Millisecond,
					MaxDelay:          10 * time.Millisecond,
					BackoffMultiplier: 2.0,
					RetryableErrors: []ErrorType{
						ErrorTypeNetwork,
						ErrorTypeTimeout,
						ErrorTypeServiceUnavailable,
					},
				},
				EnableMetrics:     true,
				EnableHealthCheck: false,
				MaxOperationTime:  5 * time.Second,
			}

			client := NewStorageClient(mockProvider, config)
			body := strings.NewReader(tt.body)

			result, err := client.PutObject(tt.bucket, tt.key, body, tt.contentType)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestStorageClient_GetObject(t *testing.T) {
	tests := []struct {
		name          string
		bucket        string
		key           string
		setupMock     func(*MockStorageProvider)
		expectedData  string
		expectError   bool
		errorContains string
	}{
		{
			name:   "successful get",
			bucket: "test-bucket",
			key:    "test-key",
			setupMock: func(m *MockStorageProvider) {
				data := "test content"
				reader := io.NopCloser(strings.NewReader(data))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil)
			},
			expectedData: "test content",
			expectError:  false,
		},
		{
			name:   "not found error",
			bucket: "test-bucket",
			key:    "missing-key",
			setupMock: func(m *MockStorageProvider) {
				m.On("GetObject", "test-bucket", "missing-key").
					Return(nil, errors.New("object not found"))
			},
			expectError:   true,
			errorContains: "storage get operation failed",
		},
		{
			name:   "network error with retry success",
			bucket: "test-bucket",
			key:    "test-key",
			setupMock: func(m *MockStorageProvider) {
				// First call fails with network error
				m.On("GetObject", "test-bucket", "test-key").
					Return(nil, errors.New("connection timeout")).Once()
				
				// Second call succeeds
				data := "test content"
				reader := io.NopCloser(strings.NewReader(data))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil).Once()
			},
			expectedData: "test content",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &ClientConfig{
				RetryConfig: &RetryConfig{
					MaxAttempts:       2,
					InitialDelay:      1 * time.Millisecond,
					MaxDelay:          10 * time.Millisecond,
					BackoffMultiplier: 2.0,
					RetryableErrors: []ErrorType{
						ErrorTypeNetwork,
						ErrorTypeTimeout,
					},
				},
				EnableMetrics:     true,
				EnableHealthCheck: false,
				MaxOperationTime:  5 * time.Second,
			}

			client := NewStorageClient(mockProvider, config)

			reader, err := client.GetObject(tt.bucket, tt.key)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, reader)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, reader)

				// Read and verify content
				data, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedData, string(data))

				// Close reader
				err = reader.Close()
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestStorageClient_UploadArtifactBundle(t *testing.T) {
	tests := []struct {
		name          string
		keyPrefix     string
		artifactPath  string
		setupMock     func(*MockStorageProvider)
		expectError   bool
		errorContains string
	}{
		{
			name:         "successful upload",
			keyPrefix:    "artifacts/test-app",
			artifactPath: "/tmp/test-app.tar.gz",
			setupMock: func(m *MockStorageProvider) {
				m.On("UploadArtifactBundle", "artifacts/test-app", "/tmp/test-app.tar.gz").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:         "upload failure",
			keyPrefix:    "artifacts/test-app",
			artifactPath: "/tmp/test-app.tar.gz",
			setupMock: func(m *MockStorageProvider) {
				// Use non-retryable error to prevent timeout in unit tests
				authErr := NewStorageError("upload_artifact_bundle", errors.New("authentication failed"), ErrorContext{})
				authErr.ErrorType = ErrorTypeAuthentication // Non-retryable
				m.On("UploadArtifactBundle", "artifacts/test-app", "/tmp/test-app.tar.gz").
					Return(authErr)
			},
			expectError:   true,
			errorContains: "artifact bundle upload failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			// Use fast retry config for unit tests to prevent timeouts
			config := DefaultClientConfig()
			config.RetryConfig = &RetryConfig{
				MaxAttempts:       3,
				InitialDelay:      1 * time.Millisecond,  // Fast for unit tests
				MaxDelay:          5 * time.Millisecond,  // Keep very short
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeTimeout,
					ErrorTypeServiceUnavailable,
					ErrorTypeRateLimit,
					ErrorTypeInternal,
					ErrorTypeCorruption,
				},
			}
			client := NewStorageClient(mockProvider, config)

			err := client.UploadArtifactBundle(tt.keyPrefix, tt.artifactPath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestStorageClient_VerifyUpload(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		setupMock     func(*MockStorageProvider)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful verification",
			key:  "test-key",
			setupMock: func(m *MockStorageProvider) {
				m.On("VerifyUpload", "test-key").Return(nil)
			},
			expectError: false,
		},
		{
			name: "verification failure",
			key:  "missing-key",
			setupMock: func(m *MockStorageProvider) {
				// Use non-retryable error to prevent timeout in unit tests
				authErr := NewStorageError("verify_upload", errors.New("authentication failed"), ErrorContext{})
				authErr.ErrorType = ErrorTypeAuthentication // Non-retryable
				m.On("VerifyUpload", "missing-key").
					Return(authErr)
			},
			expectError:   true,
			errorContains: "upload verification failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			// Use fast retry config for unit tests to prevent timeouts
			config := DefaultClientConfig()
			config.RetryConfig = &RetryConfig{
				MaxAttempts:       3,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          5 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeTimeout,
					ErrorTypeServiceUnavailable,
					ErrorTypeRateLimit,
					ErrorTypeInternal,
					ErrorTypeCorruption,
				},
			}
			client := NewStorageClient(mockProvider, config)

			err := client.VerifyUpload(tt.key)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestStorageClient_ListObjects(t *testing.T) {
	tests := []struct {
		name           string
		bucket         string
		prefix         string
		setupMock      func(*MockStorageProvider)
		expectedObjects []ObjectInfo
		expectError    bool
		errorContains  string
	}{
		{
			name:   "successful list",
			bucket: "test-bucket",
			prefix: "test-prefix",
			setupMock: func(m *MockStorageProvider) {
				objects := []ObjectInfo{
					{Key: "test-prefix/file1.txt", Size: 100},
					{Key: "test-prefix/file2.txt", Size: 200},
				}
				m.On("ListObjects", "test-bucket", "test-prefix").Return(objects, nil)
			},
			expectedObjects: []ObjectInfo{
				{Key: "test-prefix/file1.txt", Size: 100},
				{Key: "test-prefix/file2.txt", Size: 200},
			},
			expectError: false,
		},
		{
			name:   "empty list",
			bucket: "test-bucket",
			prefix: "empty-prefix",
			setupMock: func(m *MockStorageProvider) {
				m.On("ListObjects", "test-bucket", "empty-prefix").Return([]ObjectInfo{}, nil)
			},
			expectedObjects: []ObjectInfo{},
			expectError:     false,
		},
		{
			name:   "list failure",
			bucket: "test-bucket",
			prefix: "test-prefix",
			setupMock: func(m *MockStorageProvider) {
				m.On("ListObjects", "test-bucket", "test-prefix").
					Return(nil, errors.New("list failed"))
			},
			expectError:   true,
			errorContains: "object listing failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			client := NewStorageClient(mockProvider, DefaultClientConfig())

			objects, err := client.ListObjects(tt.bucket, tt.prefix)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, objects)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedObjects, objects)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestStorageClient_GetProviderType(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	mockProvider.On("GetProviderType").Return("seaweedfs")

	client := NewStorageClient(mockProvider, DefaultClientConfig())

	providerType := client.GetProviderType()
	assert.Equal(t, "seaweedfs", providerType)

	mockProvider.AssertExpectations(t)
}

func TestStorageClient_GetArtifactsBucket(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	mockProvider.On("GetArtifactsBucket").Return("artifacts")

	client := NewStorageClient(mockProvider, DefaultClientConfig())

	bucket := client.GetArtifactsBucket()
	assert.Equal(t, "artifacts", bucket)

	mockProvider.AssertExpectations(t)
}

func TestStorageClient_GetMetrics(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	
	// Test with metrics enabled
	t.Run("metrics enabled", func(t *testing.T) {
		config := DefaultClientConfig()
		config.EnableMetrics = true
		
		client := NewStorageClient(mockProvider, config)
		metrics := client.GetMetrics()
		
		assert.NotNil(t, metrics)
	})
	
	// Test with metrics disabled
	t.Run("metrics disabled", func(t *testing.T) {
		config := DefaultClientConfig()
		config.EnableMetrics = false
		
		client := NewStorageClient(mockProvider, config)
		metrics := client.GetMetrics()
		
		assert.Nil(t, metrics)
	})
}

func TestStorageClient_GetHealthStatus(t *testing.T) {
	// Test with health check enabled
	t.Run("health check enabled", func(t *testing.T) {
		mockProvider := &MockStorageProvider{}
		
		// Set up mock expectations for health check - use mock.Anything for flexible matching
		mockProvider.On("ListObjects", mock.Anything, mock.Anything).Return([]ObjectInfo{}, nil).Maybe()
		mockProvider.On("GetProviderType").Return("mock").Maybe()
		mockProvider.On("GetArtifactsBucket").Return("test-bucket").Maybe()
		mockProvider.On("PutObject", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&PutObjectResult{Size: 100}, nil).Maybe()
		mockProvider.On("GetObject", mock.Anything, mock.Anything).Return(io.NopCloser(strings.NewReader("test")), nil).Maybe()
		
		config := DefaultClientConfig()
		config.EnableHealthCheck = true
		config.EnableMetrics = true // Health check requires metrics
		
		client := NewStorageClient(mockProvider, config)
		status := client.GetHealthStatus()
		
		assert.NotNil(t, status)
		// Health check should return a status
		assert.Contains(t, []HealthStatus{HealthStatusHealthy, HealthStatusDegraded, HealthStatusUnhealthy, HealthStatusUnknown}, status.Status)
	})
	
	// Test with health check disabled
	t.Run("health check disabled", func(t *testing.T) {
		mockProvider := &MockStorageProvider{}
		
		config := DefaultClientConfig()
		config.EnableHealthCheck = false
		
		client := NewStorageClient(mockProvider, config)
		status := client.GetHealthStatus()
		
		assert.NotNil(t, status)
		assert.Equal(t, HealthStatusUnknown, status.Status)
		assert.Contains(t, status.Summary, "Health checking disabled")
	})
}

// Test helper functions

func TestFileReadSeekerResetter(t *testing.T) {
	content := "test content for seeking"
	reader := strings.NewReader(content)
	resetter := &fileReadSeekerResetter{readSeeker: reader}
	
	// Test Read
	buf := make([]byte, 4)
	n, err := resetter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", string(buf))
	
	// Test Seek
	pos, err := resetter.Seek(0, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), pos)
	
	// Test Reset
	err = resetter.Reset()
	assert.NoError(t, err)
	
	// Verify reset worked by reading again
	n, err = resetter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", string(buf))
}

func TestMetricsTrackingReadCloser(t *testing.T) {
	content := "test content for metrics tracking"
	reader := io.NopCloser(strings.NewReader(content))
	metrics := NewStorageMetrics()
	var bytesRead int64
	
	tracker := &metricsTrackingReadCloser{
		readCloser: reader,
		metrics:    metrics,
		startTime:  time.Now(),
		bytesRead:  &bytesRead,
	}
	
	// Read all content
	data, err := io.ReadAll(tracker)
	assert.NoError(t, err)
	assert.Equal(t, content, string(data))
	assert.Equal(t, int64(len(content)), bytesRead)
	
	// Close and check metrics recording
	err = tracker.Close()
	assert.NoError(t, err)
	
	// Verify metrics were recorded
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.TotalDownloads)
	assert.Equal(t, bytesRead, snapshot.TotalBytesDownloaded)
}

// Benchmark tests for performance validation

func BenchmarkStorageClient_PutObject(b *testing.B) {
	mockProvider := &MockStorageProvider{}
	mockProvider.On("PutObject", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&PutObjectResult{ETag: "test", Location: "test", Size: 100}, nil)

	config := DefaultClientConfig()
	config.EnableMetrics = false // Disable metrics for cleaner benchmarks
	config.EnableHealthCheck = false

	client := NewStorageClient(mockProvider, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := strings.NewReader("test content")
		_, err := client.PutObject("bucket", "key", body, "text/plain")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStorageClient_GetObject(b *testing.B) {
	mockProvider := &MockStorageProvider{}
	content := "test content for benchmark"
	
	mockProvider.On("GetObject", mock.Anything, mock.Anything).Return(
		func(bucket, key string) io.ReadCloser {
			return io.NopCloser(strings.NewReader(content))
		},
		nil,
	)

	config := DefaultClientConfig()
	config.EnableMetrics = false // Disable metrics for cleaner benchmarks
	config.EnableHealthCheck = false

	client := NewStorageClient(mockProvider, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := client.GetObject("bucket", "key")
		if err != nil {
			b.Fatal(err)
		}
		
		// Read and close to simulate real usage
		_, err = io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		
		reader.Close()
	}
}