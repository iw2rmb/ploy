package mocks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/stretchr/testify/mock"
)

// StorageClient is a unified mock implementation for storage operations.
// It consolidates all duplicate MockStorageClient implementations from:
// - internal/testutil/mocks.go
// - internal/testutils/mocks/storage_mock.go
// - api/server/handlers_test.go
type StorageClient struct {
	mock.Mock
	mu               sync.RWMutex
	files            map[string][]byte
	healthStatus     interface{}
	metrics          interface{}
	artifactsBucket  string
	uploadArtifactErr error
}

// NewStorageClient creates a new mock storage client with sensible defaults
func NewStorageClient() *StorageClient {
	return &StorageClient{
		files:           make(map[string][]byte),
		healthStatus:    map[string]interface{}{"status": "healthy"},
		metrics:         map[string]interface{}{"requests": 0, "errors": 0},
		artifactsBucket: "artifacts",
	}
}

// Upload uploads data to storage (supports io.Reader)
func (m *StorageClient) Upload(ctx context.Context, key string, data io.Reader) error {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(ctx, key, data)
		if args.Error(0) != nil {
			return args.Error(0)
		}
	}
	
	// Perform actual storage for realistic behavior
	m.mu.Lock()
	defer m.mu.Unlock()
	
	content, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}
	
	m.files[key] = content
	return nil
}

// Download downloads data from storage (returns io.ReadCloser)
func (m *StorageClient) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(ctx, key)
		if args.Error(1) != nil {
			return nil, args.Error(1)
		}
		if args.Get(0) != nil {
			return args.Get(0).(io.ReadCloser), nil
		}
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	content, exists := m.files[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	
	return io.NopCloser(bytes.NewReader(content)), nil
}

// Delete deletes data from storage
func (m *StorageClient) Delete(ctx context.Context, key string) error {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(ctx, key)
		if args.Error(0) != nil {
			return args.Error(0)
		}
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.files, key)
	return nil
}

// Exists checks if a key exists in storage
func (m *StorageClient) Exists(ctx context.Context, key string) (bool, error) {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(ctx, key)
		return args.Bool(0), args.Error(1)
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.files[key]
	return exists, nil
}

// List lists all keys with the given prefix
func (m *StorageClient) List(ctx context.Context, prefix string) ([]string, error) {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(ctx, prefix)
		if args.Error(1) != nil {
			return nil, args.Error(1)
		}
		if args.Get(0) != nil {
			return args.Get(0).([]string), nil
		}
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var keys []string
	for key := range m.files {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	
	return keys, nil
}

// GetHealthStatus returns the health status of the storage
func (m *StorageClient) GetHealthStatus() interface{} {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called()
		return args.Get(0)
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthStatus
}

// GetMetrics returns storage metrics
func (m *StorageClient) GetMetrics() interface{} {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called()
		return args.Get(0)
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metrics
}

// GetArtifactsBucket returns the artifacts bucket name
func (m *StorageClient) GetArtifactsBucket() string {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called()
		return args.String(0)
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.artifactsBucket
}

// UploadArtifactBundleWithVerification uploads an artifact bundle with verification
func (m *StorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (interface{}, error) {
	// Check if mock expectations are set
	if len(m.ExpectedCalls) > 0 {
		args := m.Called(keyPrefix, artifactPath)
		return args.Get(0), args.Error(1)
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.uploadArtifactErr != nil {
		return nil, m.uploadArtifactErr
	}
	
	// Return a realistic response
	return map[string]interface{}{
		"key":      keyPrefix + "bundle.tar.gz",
		"size":     1024,
		"checksum": "abc123",
	}, nil
}

// Helper methods for test setup

// WithFile presets a file in the mock storage
func (m *StorageClient) WithFile(key string, data []byte) *StorageClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.files[key] = data
	
	// Set up mock expectations for common operations
	m.On("Download", mock.Anything, key).Return(io.NopCloser(bytes.NewReader(data)), nil).Maybe()
	m.On("Exists", mock.Anything, key).Return(true, nil).Maybe()
	
	return m
}

// WithError sets up the mock to return an error for specific key operations
func (m *StorageClient) WithError(key string, err error) *StorageClient {
	m.On("Download", mock.Anything, key).Return(nil, err).Maybe()
	m.On("Upload", mock.Anything, key, mock.Anything).Return(err).Maybe()
	m.On("Exists", mock.Anything, key).Return(false, err).Maybe()
	m.On("Delete", mock.Anything, key).Return(err).Maybe()
	
	return m
}

// WithHealthStatus sets the health status to return
func (m *StorageClient) WithHealthStatus(status interface{}) *StorageClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthStatus = status
	return m
}

// WithMetrics sets the metrics to return
func (m *StorageClient) WithMetrics(metrics interface{}) *StorageClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = metrics
	return m
}

// WithArtifactsBucket sets the artifacts bucket name
func (m *StorageClient) WithArtifactsBucket(bucket string) *StorageClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.artifactsBucket = bucket
	return m
}

// WithUploadArtifactError sets an error to return for artifact uploads
func (m *StorageClient) WithUploadArtifactError(err error) *StorageClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadArtifactErr = err
	return m
}

// Reset clears all data and mock expectations
func (m *StorageClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Clear data
	m.files = make(map[string][]byte)
	
	// Reset to defaults
	m.healthStatus = map[string]interface{}{"status": "healthy"}
	m.metrics = map[string]interface{}{"requests": 0, "errors": 0}
	m.artifactsBucket = "artifacts"
	m.uploadArtifactErr = nil
	
	// Clear mock expectations
	m.ExpectedCalls = nil
	m.Calls = nil
}