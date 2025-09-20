package lifecycle

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/storage"
)

// MockUnifiedStorage is a mock implementation of storage.Storage interface
type MockUnifiedStorage struct {
	mock.Mock
}

func (m *MockUnifiedStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockUnifiedStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	args := m.Called(ctx, key, reader, opts)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockUnifiedStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.Object), args.Error(1)
}

func (m *MockUnifiedStorage) DeleteBatch(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Object), args.Error(1)
}

func (m *MockUnifiedStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	args := m.Called(ctx, key, metadata)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Copy(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Move(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockUnifiedStorage) Metrics() *storage.StorageMetrics {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*storage.StorageMetrics)
}

// MockEnvStore is a mock implementation of envstore.EnvStoreInterface
type MockEnvStore struct {
	mock.Mock
}

func (m *MockEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(envstore.AppEnvVars), args.Error(1)
}

func (m *MockEnvStore) Set(app, key, value string) error {
	args := m.Called(app, key, value)
	return args.Error(0)
}

func (m *MockEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	args := m.Called(app, envVars)
	return args.Error(0)
}

func (m *MockEnvStore) Get(app, key string) (string, bool, error) {
	args := m.Called(app, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

func (m *MockEnvStore) Delete(app, key string) error {
	args := m.Called(app, key)
	return args.Error(0)
}

func (m *MockEnvStore) ToStringArray(app string) ([]string, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// TestDestroyAppWithUnifiedStorage tests that DestroyApp can work with unified storage
func TestDestroyAppWithUnifiedStorage(t *testing.T) {
	// RED phase: This test expects DestroyApp to accept storage.Storage interface
	// It will fail initially as DestroyApp only accepts *storage.StorageClient

	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := new(MockEnvStore)

	// Mock environment store response
	envVars := envstore.AppEnvVars{
		"TEST_VAR": "test_value",
	}
	mockEnvStore.On("GetAll", "test-app").Return(envVars, nil)
	mockEnvStore.On("Delete", "test-app", "TEST_VAR").Return(nil)

	// Mock storage List operation for artifact cleanup
	mockStorage.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
		return opts.Prefix == "apps/test-app/"
	})).Return([]storage.Object{
		{Key: "apps/test-app/artifact1.tar"},
		{Key: "apps/test-app/artifact2.tar"},
	}, nil)

	// Mock storage Delete operations
	mockStorage.On("Delete", mock.Anything, "apps/test-app/artifact1.tar").Return(nil)
	mockStorage.On("Delete", mock.Anything, "apps/test-app/artifact2.tar").Return(nil)

	// Create test fiber app
	app := fiber.New()
	app.Delete("/apps/:app", func(c *fiber.Ctx) error {
		// This will fail in RED phase as DestroyAppWithStorage doesn't exist yet
		return DestroyAppWithStorage(c, mockStorage, mockEnvStore)
	})

	// Create test request
	req := httptest.NewRequest("DELETE", "/apps/test-app", nil)

	// Execute request (this will fail in RED phase)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Check response
	assert.Equal(t, 200, resp.StatusCode)

	// Verify mocks were called
	mockEnvStore.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

// TestDestroyStorageArtifactsWithUnifiedStorage tests storage artifact cleanup with unified interface
func TestDestroyStorageArtifactsWithUnifiedStorage(t *testing.T) {
	// RED phase: Test that storage artifacts can be destroyed using unified storage

	mockStorage := new(MockUnifiedStorage)

	// Mock List operation to find artifacts
	artifacts := []storage.Object{
		{Key: "apps/myapp/v1/bundle.tar", Size: 1024},
		{Key: "apps/myapp/v2/bundle.tar", Size: 2048},
		{Key: "apps/myapp/metadata.json", Size: 256},
	}
	mockStorage.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
		return opts.Prefix == "apps/myapp/"
	})).Return(artifacts, nil)

	// Mock Delete operations for each artifact
	for _, artifact := range artifacts {
		mockStorage.On("Delete", mock.Anything, artifact.Key).Return(nil)
	}

	status := map[string]interface{}{
		"operations": map[string]string{},
		"errors":     []string{},
	}

	// This function doesn't exist yet but should be implemented
	err := destroyStorageArtifactsWithUnifiedStorage("myapp", status, mockStorage)
	assert.NoError(t, err)

	// Verify all artifacts were deleted
	mockStorage.AssertExpectations(t)

	// Check status was updated
	operations := status["operations"].(map[string]string)
	assert.Equal(t, "deleted_3_artifacts", operations["storage"])
}

// TestDestroyAppDualStorageSupport tests that both storage interfaces can be used
func TestDestroyAppDualStorageSupport(t *testing.T) {
	// Test that the handler can work with either unified or legacy storage

	mockStorage := new(MockUnifiedStorage)
	mockEnvStore := new(MockEnvStore)

	// Mock environment store
	mockEnvStore.On("GetAll", "dual-app").Return(envstore.AppEnvVars{}, nil)

	// Mock unified storage operations
	mockStorage.On("List", mock.Anything, mock.MatchedBy(func(opts storage.ListOptions) bool {
		return opts.Prefix == "apps/dual-app/"
	})).Return([]storage.Object{}, nil)

	// Create test fiber app
	app := fiber.New()
	app.Delete("/apps/:app", func(c *fiber.Ctx) error {
		// Test with DestroyAppWithStorage (new function with unified storage)
		return DestroyAppWithStorage(c, mockStorage, mockEnvStore)
	})

	// Create test request
	req := httptest.NewRequest("DELETE", "/apps/dual-app", nil)

	// Execute request (will fail in RED phase as function doesn't exist)
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Check response
	assert.Equal(t, 200, resp.StatusCode)

	// Verify mocks were called
	mockEnvStore.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}
