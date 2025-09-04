package server

import (
    "context"
    "io"
    "testing"

    "github.com/iw2rmb/ploy/internal/storage"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of storage.Storage interface
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

func (m *MockStorage) Metrics() storage.StorageMetrics {
	args := m.Called()
	return args.Get(0).(storage.StorageMetrics)
}

// Test that getStorageClient uses the new factory pattern
func TestServer_GetStorageClient_RequiresConfigService(t *testing.T) {
    server := &Server{dependencies: &ServiceDependencies{}}
    storage, err := server.getStorageClient()
    assert.Error(t, err)
    assert.Nil(t, storage)
}

// Test successful storage creation with new factory
// Legacy migration tests removed; covered by storage_service tests
