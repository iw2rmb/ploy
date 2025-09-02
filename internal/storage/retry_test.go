package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRetryWithBackoff(t *testing.T) {
	tests := []struct {
		name          string
		operation     func() RetryOperation
		config        *RetryConfig
		operationName string
		expectError   bool
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

func TestNewRetryableStorageClient(t *testing.T) {
	mockProvider := &MockStorageProvider{}

	t.Run("with default config", func(t *testing.T) {
		client := NewRetryableStorageClient(mockProvider, nil)

		assert.NotNil(t, client)
		assert.Equal(t, mockProvider, client.client)
		assert.NotNil(t, client.config)
		assert.Equal(t, 3, client.config.MaxAttempts) // Default config
	})

	t.Run("with custom config", func(t *testing.T) {
		customConfig := &RetryConfig{
			MaxAttempts:       5,
			InitialDelay:      500 * time.Millisecond,
			MaxDelay:          10 * time.Second,
			BackoffMultiplier: 1.5,
		}

		client := NewRetryableStorageClient(mockProvider, customConfig)

		assert.NotNil(t, client)
		assert.Equal(t, mockProvider, client.client)
		assert.Equal(t, customConfig, client.config)
		assert.Equal(t, 5, client.config.MaxAttempts)
	})
}

// Mock ReadSeekerResetter for testing
type MockReadSeekerResetter struct {
	mock.Mock
	*strings.Reader
}

func NewMockReadSeekerResetter(content string) *MockReadSeekerResetter {
	return &MockReadSeekerResetter{
		Reader: strings.NewReader(content),
	}
}

func (m *MockReadSeekerResetter) Reset() error {
	args := m.Called()
	// Actually reset the reader
	m.Reader = strings.NewReader("test content")
	return args.Error(0)
}

func TestRetryableStorageClient_PutObject(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*MockStorageProvider)
		bucket        string
		key           string
		contentType   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful put",
			bucket:      "test-bucket",
			key:         "test-key",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(&PutObjectResult{
						ETag:     "test-etag",
						Location: "test-location",
						Size:     12,
					}, nil)
			},
			expectError: false,
		},
		{
			name:        "retry on network error then success",
			bucket:      "test-bucket",
			key:         "test-key",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				// First attempt fails
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(nil, errors.New("connection refused")).Once()
				// Second attempt succeeds
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(&PutObjectResult{
						ETag:     "test-etag",
						Location: "test-location",
						Size:     12,
					}, nil).Once()
			},
			expectError: false,
		},
		{
			name:        "non-retryable error",
			bucket:      "test-bucket",
			key:         "test-key",
			contentType: "text/plain",
			setupMock: func(m *MockStorageProvider) {
				m.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").
					Return(nil, errors.New("authentication failed"))
			},
			expectError:   true,
			errorContains: "authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeInternal,
				},
			}

			client := NewRetryableStorageClient(mockProvider, config)

			body := NewMockReadSeekerResetter("test content")
			body.On("Reset").Return(nil)

			result, err := client.PutObject(tt.bucket, tt.key, body, tt.contentType)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			mockProvider.AssertExpectations(t)
			body.AssertExpectations(t)
		})
	}
}

func TestRetryableStorageClient_GetObject(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*MockStorageProvider)
		bucket        string
		key           string
		expectError   bool
		errorContains string
	}{
		{
			name:   "successful get",
			bucket: "test-bucket",
			key:    "test-key",
			setupMock: func(m *MockStorageProvider) {
				reader := io.NopCloser(strings.NewReader("test content"))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil)
			},
			expectError: false,
		},
		{
			name:   "retry on network error then success",
			bucket: "test-bucket",
			key:    "test-key",
			setupMock: func(m *MockStorageProvider) {
				// First attempt fails
				m.On("GetObject", "test-bucket", "test-key").
					Return(nil, errors.New("connection timeout")).Once()
				// Second attempt succeeds
				reader := io.NopCloser(strings.NewReader("test content"))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil).Once()
			},
			expectError: false,
		},
		{
			name:   "permanent failure",
			bucket: "test-bucket",
			key:    "missing-key",
			setupMock: func(m *MockStorageProvider) {
				m.On("GetObject", "test-bucket", "missing-key").
					Return(nil, errors.New("object not found"))
			},
			expectError:   true,
			errorContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeTimeout,
					ErrorTypeInternal,
				},
			}

			client := NewRetryableStorageClient(mockProvider, config)

			reader, err := client.GetObject(tt.bucket, tt.key)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, reader)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, reader)
				
				// Test reading from the retryable reader
				data, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, "test content", string(data))
				
				// Close the reader
				err = reader.Close()
				assert.NoError(t, err)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestRetryableStorageClient_UploadArtifactBundle(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*MockStorageProvider)
		keyPrefix     string
		artifactPath  string
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
			name:         "retry on service unavailable then success",
			keyPrefix:    "artifacts/test-app",
			artifactPath: "/tmp/test-app.tar.gz",
			setupMock: func(m *MockStorageProvider) {
				// First attempt fails
				m.On("UploadArtifactBundle", "artifacts/test-app", "/tmp/test-app.tar.gz").
					Return(errors.New("service unavailable")).Once()
				// Second attempt succeeds
				m.On("UploadArtifactBundle", "artifacts/test-app", "/tmp/test-app.tar.gz").
					Return(nil).Once()
			},
			expectError: false,
		},
		{
			name:         "permanent failure",
			keyPrefix:    "artifacts/test-app",
			artifactPath: "/invalid/path",
			setupMock: func(m *MockStorageProvider) {
				m.On("UploadArtifactBundle", "artifacts/test-app", "/invalid/path").
					Return(errors.New("file not found"))
			},
			expectError:   true,
			errorContains: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeServiceUnavailable,
					ErrorTypeInternal,
				},
			}

			client := NewRetryableStorageClient(mockProvider, config)

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

func TestRetryableStorageClient_VerifyUpload(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*MockStorageProvider)
		key           string
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
			name: "retry on timeout then success",
			key:  "test-key",
			setupMock: func(m *MockStorageProvider) {
				// First attempt fails
				m.On("VerifyUpload", "test-key").
					Return(errors.New("timeout")).Once()
				// Second attempt succeeds
				m.On("VerifyUpload", "test-key").Return(nil).Once()
			},
			expectError: false,
		},
		{
			name: "permanent failure",
			key:  "missing-key",
			setupMock: func(m *MockStorageProvider) {
				m.On("VerifyUpload", "missing-key").
					Return(errors.New("key not found"))
			},
			expectError:   true,
			errorContains: "key not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeTimeout,
					ErrorTypeInternal,
				},
			}

			client := NewRetryableStorageClient(mockProvider, config)

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

func TestRetryableStorageClient_ListObjects(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*MockStorageProvider)
		bucket         string
		prefix         string
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
			name:   "retry on network error then success",
			bucket: "test-bucket",
			prefix: "test-prefix",
			setupMock: func(m *MockStorageProvider) {
				// First attempt fails
				m.On("ListObjects", "test-bucket", "test-prefix").
					Return(nil, errors.New("network unreachable")).Once()
				// Second attempt succeeds
				objects := []ObjectInfo{{Key: "test-prefix/file1.txt", Size: 100}}
				m.On("ListObjects", "test-bucket", "test-prefix").Return(objects, nil).Once()
			},
			expectedObjects: []ObjectInfo{{Key: "test-prefix/file1.txt", Size: 100}},
			expectError:     false,
		},
		{
			name:   "permanent failure",
			bucket: "invalid-bucket",
			prefix: "test-prefix",
			setupMock: func(m *MockStorageProvider) {
				m.On("ListObjects", "invalid-bucket", "test-prefix").
					Return(nil, errors.New("bucket not found"))
			},
			expectError:   true,
			errorContains: "bucket not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			tt.setupMock(mockProvider)

			config := &RetryConfig{
				MaxAttempts:       2,
				InitialDelay:      1 * time.Millisecond,
				MaxDelay:          10 * time.Millisecond,
				BackoffMultiplier: 2.0,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeInternal,
				},
			}

			client := NewRetryableStorageClient(mockProvider, config)

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

func TestRetryableStorageClient_PassthroughMethods(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	mockProvider.On("GetProviderType").Return("test-provider")
	mockProvider.On("GetArtifactsBucket").Return("test-artifacts")

	client := NewRetryableStorageClient(mockProvider, DefaultRetryConfig())

	// Test GetProviderType
	providerType := client.GetProviderType()
	assert.Equal(t, "test-provider", providerType)

	// Test GetArtifactsBucket
	bucket := client.GetArtifactsBucket()
	assert.Equal(t, "test-artifacts", bucket)

	mockProvider.AssertExpectations(t)
}

// Mock ReadCloser for testing retryable reader
type MockReadCloser struct {
	mock.Mock
	*strings.Reader
	closed bool
}

func NewMockReadCloser(content string) *MockReadCloser {
	return &MockReadCloser{
		Reader: strings.NewReader(content),
		closed: false,
	}
}

func (m *MockReadCloser) Read(p []byte) (int, error) {
	if m.closed {
		return 0, errors.New("reader closed")
	}
	return m.Reader.Read(p)
}

func (m *MockReadCloser) Close() error {
	args := m.Called()
	m.closed = true
	return args.Error(0)
}

// FailingReader implements ReadCloser but fails on first read
type FailingReader struct {
	mock.Mock
	closed bool
}

func (f *FailingReader) Read(p []byte) (int, error) {
	if f.closed {
		return 0, errors.New("reader closed")
	}
	// Return a simple error with "connection reset" text to trigger retry
	return 0, errors.New("connection reset")
}

func (f *FailingReader) Close() error {
	args := f.Called()
	f.closed = true
	return args.Error(0)
}

func TestRetryableReadCloser(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*MockStorageProvider)
		content     string
		expectError bool
		expectRetry bool
	}{
		{
			name:    "successful read",
			content: "test content",
			setupMock: func(m *MockStorageProvider) {
				// No additional GetObject calls expected for successful read
			},
			expectError: false,
			expectRetry: false,
		},
		{
			name:    "read error triggers retry",
			content: "test content",
			setupMock: func(m *MockStorageProvider) {
				// Retry call to GetObject
				reader := io.NopCloser(strings.NewReader("recovered content"))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil)
			},
			expectError: false, // Should recover from error
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			if tt.setupMock != nil {
				tt.setupMock(mockProvider)
			}

			var retryableReader *retryableReadCloser
			var expectedReader ReadCloser
			
			// Setup different readers based on test scenario
			if tt.expectRetry {
				// Use a failing reader that returns a network error
				failingReader := &FailingReader{}
				failingReader.On("Close").Return(nil)
				expectedReader = failingReader
				
				retryableReader = &retryableReadCloser{
					reader: failingReader,
					client: mockProvider,
					bucket: "test-bucket",
					key:    "test-key",
					config: DefaultRetryConfig(),
				}
			} else {
				// Create normal reader
				initialReader := NewMockReadCloser(tt.content)
				initialReader.On("Close").Return(nil)
				expectedReader = initialReader
				
				retryableReader = &retryableReadCloser{
					reader: initialReader,
					client: mockProvider,
					bucket: "test-bucket",
					key:    "test-key",
					config: DefaultRetryConfig(),
				}
			}

			// Test read (either normal or retry scenario)
			bufSize := len(tt.content)
			if tt.expectRetry {
				bufSize = len("recovered content") // Make buffer big enough for recovery
			}
			buf := make([]byte, bufSize)
			
			n, err := retryableReader.Read(buf)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectRetry {
					// Should have retried and got "recovered content"
					assert.Equal(t, "recovered content", string(buf[:n]))
				} else {
					assert.Equal(t, len(tt.content), n)
					assert.Equal(t, tt.content, string(buf[:n]))
				}
			}

			// Test close
			closeErr := retryableReader.Close()
			assert.NoError(t, closeErr)

			mockProvider.AssertExpectations(t)
			
			// Assert expectations on the reader that was actually used
			if mockReader, ok := expectedReader.(interface{ AssertExpectations(*testing.T) }); ok {
				mockReader.AssertExpectations(t)
			}
		})
	}
}

func TestRetryableReadCloser_Retry(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	initialReader := NewMockReadCloser("initial content")
	retryReader := io.NopCloser(strings.NewReader("retry content"))

	initialReader.On("Close").Return(nil)
	mockProvider.On("GetObject", "test-bucket", "test-key").Return(retryReader, nil)

	retryableReader := &retryableReadCloser{
		reader: initialReader,
		client: mockProvider,
		bucket: "test-bucket",
		key:    "test-key",
		config: DefaultRetryConfig(),
	}

	// Test retry
	err := retryableReader.Retry()
	assert.NoError(t, err)

	// Verify new reader is set
	assert.NotEqual(t, initialReader, retryableReader.reader)

	// Test reading from new reader
	buf := make([]byte, 20)
	n, err := retryableReader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "retry content", string(buf[:n]))

	mockProvider.AssertExpectations(t)
	initialReader.AssertExpectations(t)
}

func TestIsRetryableReadError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout occurred"),
			expected: true,
		},
		{
			name:     "random error",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "EOF error",
			err:      io.EOF,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableReadError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
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

func BenchmarkRetryableStorageClient_PutObject(b *testing.B) {
	mockProvider := &MockStorageProvider{}
	mockProvider.On("PutObject", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&PutObjectResult{ETag: "test", Location: "test", Size: 100}, nil)

	config := &RetryConfig{
		MaxAttempts:       2,
		InitialDelay:      1 * time.Microsecond,
		MaxDelay:          10 * time.Microsecond,
		BackoffMultiplier: 2.0,
	}

	client := NewRetryableStorageClient(mockProvider, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := NewMockReadSeekerResetter("test content")
		body.On("Reset").Return(nil)
		_, _ = client.PutObject("bucket", "key", body, "text/plain")
	}
}