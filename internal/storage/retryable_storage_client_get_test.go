package storage

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
