package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
