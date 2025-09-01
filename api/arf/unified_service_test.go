package arf

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockUnifiedStorageForNoBucketPrefix tracks actual keys passed to it
type MockUnifiedStorageForNoBucketPrefix struct {
	mock.Mock
	lastPutKey    string
	lastGetKey    string
	lastDeleteKey string
	lastExistsKey string
}

func (m *MockUnifiedStorageForNoBucketPrefix) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.lastGetKey = key
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	m.lastPutKey = key
	args := m.Called(ctx, key, reader)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Delete(ctx context.Context, key string) error {
	m.lastDeleteKey = key
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Exists(ctx context.Context, key string) (bool, error) {
	m.lastExistsKey = key
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockUnifiedStorageForNoBucketPrefix) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.Object), args.Error(1)
}

func (m *MockUnifiedStorageForNoBucketPrefix) DeleteBatch(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Head(ctx context.Context, key string) (*storage.Object, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Object), args.Error(1)
}

func (m *MockUnifiedStorageForNoBucketPrefix) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	args := m.Called(ctx, key, metadata)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Copy(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Move(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockUnifiedStorageForNoBucketPrefix) Metrics() *storage.StorageMetrics {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*storage.StorageMetrics)
}

// TestARFServiceDoesNotAddBucketPrefix verifies that ARFService passes keys without modification
// According to the storage fix plan, ARFService should NOT add bucket prefixes
func TestARFServiceDoesNotAddBucketPrefix(t *testing.T) {
	mockStorage := &MockUnifiedStorageForNoBucketPrefix{}
	ctx := context.Background()

	// Create ARFService with "artifacts" bucket
	// Even though bucket is specified, it should NOT be added to keys
	service, err := NewARFService(mockStorage, "artifacts")
	require.NoError(t, err)
	require.NotNil(t, service)

	t.Run("Put should not add bucket prefix", func(t *testing.T) {
		testKey := "jobs/123/input.tar"
		testData := []byte("test data")

		// Mock expects Put to be called with the exact key, no prefix
		mockStorage.On("Put", ctx, testKey, mock.Anything).Return(nil).Once()

		err := service.Put(ctx, testKey, testData)
		assert.NoError(t, err)

		// Verify the actual key passed to storage has NO bucket prefix
		assert.Equal(t, testKey, mockStorage.lastPutKey, "ARFService should pass key without modification")
		mockStorage.AssertExpectations(t)
	})

	t.Run("Get should not add bucket prefix", func(t *testing.T) {
		testKey := "jobs/456/output.tar"
		testData := []byte("result data")

		// Mock expects Get to be called with the exact key, no prefix
		mockStorage.On("Get", ctx, testKey).Return(
			io.NopCloser(bytes.NewReader(testData)), nil,
		).Once()

		data, err := service.Get(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, testData, data)

		// Verify the actual key passed to storage has NO bucket prefix
		assert.Equal(t, testKey, mockStorage.lastGetKey, "ARFService should pass key without modification")
		mockStorage.AssertExpectations(t)
	})

	t.Run("Delete should not add bucket prefix", func(t *testing.T) {
		testKey := "jobs/789/temp.tar"

		// Mock expects Delete to be called with the exact key, no prefix
		mockStorage.On("Delete", ctx, testKey).Return(nil).Once()

		err := service.Delete(ctx, testKey)
		assert.NoError(t, err)

		// Verify the actual key passed to storage has NO bucket prefix
		assert.Equal(t, testKey, mockStorage.lastDeleteKey, "ARFService should pass key without modification")
		mockStorage.AssertExpectations(t)
	})

	t.Run("Exists should not add bucket prefix", func(t *testing.T) {
		testKey := "jobs/999/check.tar"

		// Mock expects Exists to be called with the exact key, no prefix
		mockStorage.On("Exists", ctx, testKey).Return(true, nil).Once()

		exists, err := service.Exists(ctx, testKey)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Verify the actual key passed to storage has NO bucket prefix
		assert.Equal(t, testKey, mockStorage.lastExistsKey, "ARFService should pass key without modification")
		mockStorage.AssertExpectations(t)
	})
}

// TestARFServiceWithEmptyBucket verifies behavior when bucket is empty
func TestARFServiceWithEmptyBucket(t *testing.T) {
	mockStorage := &MockUnifiedStorageForNoBucketPrefix{}
	ctx := context.Background()

	// Create ARFService with empty bucket
	service, err := NewARFService(mockStorage, "")
	require.NoError(t, err)
	require.NotNil(t, service)

	testKey := "recipes/openrewrite.yaml"
	testData := []byte("recipe content")

	// Mock expects Put to be called with the exact key
	mockStorage.On("Put", ctx, testKey, mock.Anything).Return(nil).Once()

	err = service.Put(ctx, testKey, testData)
	assert.NoError(t, err)

	// Verify the key is passed without any prefix
	assert.Equal(t, testKey, mockStorage.lastPutKey, "ARFService should pass key without modification when bucket is empty")
	mockStorage.AssertExpectations(t)
}

