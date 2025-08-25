// Package mocks provides mock implementations for testing
package mocks

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockStorageClient is a mock implementation of storage.Client
type MockStorageClient struct {
	mock.Mock
	mu    sync.RWMutex
	files map[string][]byte
}

// NewMockStorageClient creates a new mock storage client
func NewMockStorageClient() *MockStorageClient {
	return &MockStorageClient{
		files: make(map[string][]byte),
	}
}

// Store stores data with the given key
func (m *MockStorageClient) Store(ctx context.Context, key string, data io.Reader) error {
	args := m.Called(ctx, key, data)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	// Actually store the data for realistic behavior
	m.mu.Lock()
	defer m.mu.Unlock()

	content, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	m.files[key] = content
	return nil
}

// Retrieve retrieves data by key
func (m *MockStorageClient) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	content, exists := m.files[key]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", key)
	}

	return &mockReadCloser{content: content}, nil
}

// Delete deletes data by key
func (m *MockStorageClient) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.files, key)
	return nil
}

// Exists checks if a key exists
func (m *MockStorageClient) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	
	if args.Error(1) != nil {
		return false, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.files[key]
	return exists, nil
}

// List lists all keys with the given prefix
func (m *MockStorageClient) List(ctx context.Context, prefix string) ([]string, error) {
	args := m.Called(ctx, prefix)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for key := range m.files {
		if len(prefix) == 0 || key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// GetURL returns a URL for accessing the stored data
func (m *MockStorageClient) GetURL(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	
	if args.Error(1) != nil {
		return "", args.Error(1)
	}

	return fmt.Sprintf("http://mock-storage:8888/%s", key), nil
}

// Close closes the storage client
func (m *MockStorageClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Health checks the health of the storage service
func (m *MockStorageClient) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// SetupDefault sets up default mock behavior for common operations
func (m *MockStorageClient) SetupDefault() {
	m.On("Store", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string"), mock.Anything).Return(nil)
	m.On("Retrieve", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string")).Return(&mockReadCloser{}, nil)
	m.On("Delete", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string")).Return(nil)
	m.On("Exists", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string")).Return(true, nil)
	m.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string")).Return([]string{}, nil)
	m.On("GetURL", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("string")).Return("http://mock-storage:8888/test", nil)
	m.On("Close").Return(nil)
	m.On("Health", mock.AnythingOfType("*context.timerCtx")).Return(nil)
}

// SimulateFailure configures the mock to simulate storage failures
func (m *MockStorageClient) SimulateFailure(operation string, err error) {
	switch operation {
	case "store":
		m.On("Store", mock.Anything, mock.Anything, mock.Anything).Return(err)
	case "retrieve":
		m.On("Retrieve", mock.Anything, mock.Anything).Return(nil, err)
	case "delete":
		m.On("Delete", mock.Anything, mock.Anything).Return(err)
	case "health":
		m.On("Health", mock.Anything).Return(err)
	}
}

// SimulateLatency adds artificial latency to operations
func (m *MockStorageClient) SimulateLatency(duration time.Duration) {
	m.On("Store", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { time.Sleep(duration) }).
		Return(nil)
	
	m.On("Retrieve", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { time.Sleep(duration) }).
		Return(&mockReadCloser{}, nil)
}

// GetStoredData returns the data that was stored (for testing purposes)
func (m *MockStorageClient) GetStoredData(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	data, exists := m.files[key]
	return data, exists
}

// ClearStoredData clears all stored data
func (m *MockStorageClient) ClearStoredData() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.files = make(map[string][]byte)
}

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	content []byte
	pos     int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.content) {
		return 0, io.EOF
	}
	
	n = copy(p, m.content[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}