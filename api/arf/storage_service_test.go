package arf

import (
	"bytes"
	"context"
	"errors"
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

func (m *MockStorage) Metrics() *storage.StorageMetrics {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*storage.StorageMetrics)
}

// Test StorageAdapter Put method
func TestStorageAdapter_Put(t *testing.T) {
	mockStorage := new(MockStorage)
	adapter := NewStorageAdapter(mockStorage)

	ctx := context.Background()
	testData := []byte("test data")

	// Mock expects Put to be called with the right parameters
	mockStorage.On("Put", ctx, "arf-recipes/test-key", mock.Anything).Return(nil)

	err := adapter.Put(ctx, "test-key", testData)
	assert.NoError(t, err)

	mockStorage.AssertExpectations(t)
}

// Test StorageAdapter Get method
func TestStorageAdapter_Get(t *testing.T) {
	mockStorage := new(MockStorage)
	adapter := NewStorageAdapter(mockStorage)

	ctx := context.Background()
	testData := []byte("test data")

	// Create a mock ReadCloser
	reader := io.NopCloser(bytes.NewReader(testData))
	mockStorage.On("Get", ctx, "arf-recipes/test-key").Return(reader, nil)

	data, err := adapter.Get(ctx, "test-key")
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	mockStorage.AssertExpectations(t)
}

// Test StorageAdapter Delete method
func TestStorageAdapter_Delete(t *testing.T) {
	mockStorage := new(MockStorage)
	adapter := NewStorageAdapter(mockStorage)

	ctx := context.Background()

	mockStorage.On("Delete", ctx, "arf-recipes/test-key").Return(nil)

	err := adapter.Delete(ctx, "test-key")
	assert.NoError(t, err)

	mockStorage.AssertExpectations(t)
}

// Test StorageAdapter Exists method
func TestStorageAdapter_Exists(t *testing.T) {
	mockStorage := new(MockStorage)
	adapter := NewStorageAdapter(mockStorage)

	ctx := context.Background()

	mockStorage.On("Exists", ctx, "arf-recipes/test-key").Return(true, nil)

	exists, err := adapter.Exists(ctx, "test-key")
	assert.NoError(t, err)
	assert.True(t, exists)

	mockStorage.AssertExpectations(t)
}

// Test StorageAdapter error handling
func TestStorageAdapter_ErrorHandling(t *testing.T) {
	mockStorage := new(MockStorage)
	adapter := NewStorageAdapter(mockStorage)

	ctx := context.Background()
	testErr := errors.New("storage error")

	// Test Put error
	mockStorage.On("Put", ctx, "arf-recipes/error-key", mock.Anything).Return(testErr)
	err := adapter.Put(ctx, "error-key", []byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to put key")

	// Test Get error
	mockStorage.On("Get", ctx, "arf-recipes/error-key").Return(nil, testErr)
	_, err = adapter.Get(ctx, "error-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get key")

	// Test Delete error
	mockStorage.On("Delete", ctx, "arf-recipes/error-key").Return(testErr)
	err = adapter.Delete(ctx, "error-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete key")

	// Test Exists error
	mockStorage.On("Exists", ctx, "arf-recipes/error-key").Return(false, testErr)
	_, err = adapter.Exists(ctx, "error-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check existence")

	mockStorage.AssertExpectations(t)
}
