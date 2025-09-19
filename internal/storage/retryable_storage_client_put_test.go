package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
