package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryableStorageClient_ListObjects_RetriesThenSucceeds(t *testing.T) {
	prov := &countProvider{}
	cfg := &RetryConfig{MaxAttempts: 2, InitialDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond, BackoffMultiplier: 2.0, RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeInternal, ErrorTypeServiceUnavailable}}
	c := NewRetryableStorageClient(prov, cfg)
	objs, err := c.ListObjects("bucket", "prefix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov.listCalls != 2 {
		t.Fatalf("expected 2 list calls, got %d", prov.listCalls)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
}

func TestRetryableStorageClient_ListObjects(t *testing.T) {
	tests := []struct {
		name            string
		setupMock       func(*MockStorageProvider)
		bucket          string
		prefix          string
		expectedObjects []ObjectInfo
		expectError     bool
		errorContains   string
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
