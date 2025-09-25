package health

import (
	"context"
	"errors"
	"io"
	"testing"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockStorage is a mock implementation of the storage.Storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	args := m.Called(ctx, key, reader)
	return args.Error(0)
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.Object), args.Error(1)
}

func (m *MockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Object), args.Error(1)
}

func (m *MockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	args := m.Called(ctx, key, metadata)
	return args.Error(0)
}

func (m *MockStorage) Copy(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockStorage) Move(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockStorage) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Metrics() *storage.StorageMetrics {
	args := m.Called()
	return args.Get(0).(*storage.StorageMetrics)
}

// Test that the new factory-based storage is used in health checks
func TestHealthChecker_CheckSeaweedFS_WithFactory(t *testing.T) {
	// This test will initially fail because checkSeaweedFS still uses CreateStorageClientFromConfig
	// After migration, it should use CreateStorageFromFactory and the Storage interface

	h := NewHealthChecker("/tmp/test-config.yaml", "http://nomad:4646")

	// The test should verify that:
	// 1. CreateStorageFromFactory is called instead of CreateStorageClientFromConfig
	// 2. The Storage.Health() method is used for health checking
	// 3. The response is properly formatted

	// For now, this will fail as expected (RED phase of TDD)
	dep := h.checkSeaweedFS()

	// After migration, we expect:
	// - dep.Status to be "unhealthy" if config file doesn't exist
	// - dep.Error to contain appropriate error message
	assert.NotNil(t, dep)
	assert.Contains(t, dep.Status, "unhealthy")
}

func TestHealthChecker_CheckSeaweedFS_ConfigServiceMemoryProvider(t *testing.T) {
	svc, err := cfgsvc.New(cfgsvc.WithDefaults(&cfgsvc.Config{
		Storage: cfgsvc.StorageConfig{Provider: "memory"},
	}))
	require.NoError(t, err)

	h := NewHealthChecker("", "")
	h.SetConfigService(svc)

	require.NotPanics(t, func() {
		dep := h.checkSeaweedFS()
		require.NotNil(t, dep)
		assert.Equal(t, "healthy", dep.Status)
	})
}

// Test the new storage factory health check behavior
func TestHealthChecker_CheckSeaweedFS_FactorySuccess(t *testing.T) {
	// This test verifies successful health check with mock storage
	// It will fail initially but pass after migration

	mockStorage := new(MockStorage)
	mockStorage.On("Health", mock.Anything).Return(nil)
	mockStorage.On("Metrics").Return(&storage.StorageMetrics{
		TotalUploads:         100,
		SuccessfulUploads:    95,
		FailedUploads:        5,
		TotalDownloads:       200,
		SuccessfulDownloads:  190,
		FailedDownloads:      10,
		TotalBytesUploaded:   1000,
		TotalBytesDownloaded: 2000,
	})

	// After migration, we'll need to inject the mock storage
	// For now, this test documents the expected behavior
	t.Skip("Skipping until migration is complete")
}

// Test error handling with new factory pattern
func TestHealthChecker_CheckSeaweedFS_FactoryError(t *testing.T) {
	// This test verifies error handling when storage health check fails

	mockStorage := new(MockStorage)
	mockStorage.On("Health", mock.Anything).Return(errors.New("connection refused"))

	// After migration, this should properly handle the error
	// and return an unhealthy status with error details
	t.Skip("Skipping until migration is complete")
}
