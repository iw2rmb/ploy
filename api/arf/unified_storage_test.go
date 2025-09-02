package arf

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockUnifiedStorage implements storage.Storage interface for testing
type MockUnifiedStorage struct {
	data map[string][]byte
}

func NewMockUnifiedStorage() *MockUnifiedStorage {
	return &MockUnifiedStorage{
		data: make(map[string][]byte),
	}
}

func (m *MockUnifiedStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, storage.NewStorageError("not_found", nil, storage.ErrorContext{Key: key})
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockUnifiedStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *MockUnifiedStorage) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockUnifiedStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockUnifiedStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	var objects []storage.Object
	for key := range m.data {
		if strings.HasPrefix(key, opts.Prefix) {
			objects = append(objects, storage.Object{
				Key:  key,
				Size: int64(len(m.data[key])),
			})
		}
	}
	return objects, nil
}

func (m *MockUnifiedStorage) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func (m *MockUnifiedStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	data, exists := m.data[key]
	if !exists {
		return nil, storage.NewStorageError("not_found", nil, storage.ErrorContext{Key: key})
	}
	return &storage.Object{
		Key:  key,
		Size: int64(len(data)),
	}, nil
}

func (m *MockUnifiedStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil
}

func (m *MockUnifiedStorage) Copy(ctx context.Context, src, dst string) error {
	data, exists := m.data[src]
	if !exists {
		return storage.NewStorageError("not_found", nil, storage.ErrorContext{Key: src})
	}
	m.data[dst] = make([]byte, len(data))
	copy(m.data[dst], data)
	return nil
}

func (m *MockUnifiedStorage) Move(ctx context.Context, src, dst string) error {
	if err := m.Copy(ctx, src, dst); err != nil {
		return err
	}
	return m.Delete(ctx, src)
}

func (m *MockUnifiedStorage) Health(ctx context.Context) error {
	return nil
}

func (m *MockUnifiedStorage) Metrics() *storage.StorageMetrics {
	return storage.NewStorageMetrics()
}

// TestNewARFServiceWithUnifiedStorage tests creating ARF service with unified storage interface
func TestNewARFServiceWithUnifiedStorage(t *testing.T) {
	mockStorage := NewMockUnifiedStorage()

	// ARFService now works with unified storage interface directly
	service, err := NewARFService(mockStorage)
	require.NoError(t, err)
	require.NotNil(t, service)
}

// TestARFOperationsWithUnifiedStorage tests ARF operations using unified storage
func TestARFOperationsWithUnifiedStorage(t *testing.T) {
	mockStorage := NewMockUnifiedStorage()
	ctx := context.Background()

	// ARFService now works with unified storage interface directly
	service, err := NewARFService(mockStorage)
	require.NoError(t, err)

	// Test Put operation
	testData := []byte("test recipe data")
	err = service.Put(ctx, "recipes/test-recipe.yaml", testData)
	assert.NoError(t, err)

	// Test Get operation
	data, err := service.Get(ctx, "recipes/test-recipe.yaml")
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	// Test Exists operation
	exists, err := service.Exists(ctx, "recipes/test-recipe.yaml")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test Delete operation
	err = service.Delete(ctx, "recipes/test-recipe.yaml")
	assert.NoError(t, err)

	// Verify deletion
	exists, err = service.Exists(ctx, "recipes/test-recipe.yaml")
	assert.NoError(t, err)
	assert.False(t, exists)
}

// TestBucketPrefixing tests that bucket prefixing works correctly with unified storage
func TestBucketPrefixing(t *testing.T) {
	mockStorage := NewMockUnifiedStorage()
	ctx := context.Background()

	service, err := NewARFService(mockStorage)
	require.NoError(t, err)

	// Store data - should be prefixed with bucket name
	testData := []byte("test data")
	err = service.Put(ctx, "test-key", testData)
	assert.NoError(t, err)

	// Verify the key was prefixed with bucket name in underlying storage
	prefixedKey := "artifacts/test-key"
	reader, err := mockStorage.Get(ctx, prefixedKey)
	assert.NoError(t, err)
	defer reader.Close()

	storedData, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, testData, storedData)
}
