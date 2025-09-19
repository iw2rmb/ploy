package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryableStorageClient_VerifyUpload_RetriesThenSucceeds(t *testing.T) {
	prov := &countProvider{}
	cfg := &RetryConfig{MaxAttempts: 2, InitialDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond, BackoffMultiplier: 2.0, RetryableErrors: []ErrorType{ErrorTypeServiceUnavailable, ErrorTypeInternal}}
	c := NewRetryableStorageClient(prov, cfg)
	if err := c.VerifyUpload("k"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov.verifyCalls != 2 {
		t.Fatalf("expected 2 verify calls, got %d", prov.verifyCalls)
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
