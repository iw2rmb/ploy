package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
