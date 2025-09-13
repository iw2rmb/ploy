package middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockStorage is a mock implementation of storage.Storage for testing
type MockStorage struct {
	// Control behavior
	failCount    int
	currentCalls int
	failError    error

	// Track calls
	getCalls    []string
	putCalls    []string
	deleteCalls []string
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		getCalls:    make([]string, 0),
		putCalls:    make([]string, 0),
		deleteCalls: make([]string, 0),
	}
}

func (m *MockStorage) SetFailures(count int, err error) {
	m.failCount = count
	m.failError = err
	m.currentCalls = 0
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.getCalls = append(m.getCalls, key)
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return nil, m.failError
	}

	return io.NopCloser(strings.NewReader("test content")), nil
}

func (m *MockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	m.putCalls = append(m.putCalls, key)
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	m.deleteCalls = append(m.deleteCalls, key)
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return false, m.failError
	}

	return true, nil
}

func (m *MockStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return nil, m.failError
	}

	return []storage.Object{
		{Key: "test1", Size: 100},
		{Key: "test2", Size: 200},
	}, nil
}

func (m *MockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	m.deleteCalls = append(m.deleteCalls, keys...)
	return nil
}

func (m *MockStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return nil, m.failError
	}

	return &storage.Object{
		Key:  key,
		Size: 1024,
	}, nil
}

func (m *MockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Copy(ctx context.Context, src, dst string) error {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Move(ctx context.Context, src, dst string) error {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Health(ctx context.Context) error {
	m.currentCalls++

	if m.currentCalls <= m.failCount {
		return m.failError
	}

	return nil
}

func (m *MockStorage) Metrics() *storage.StorageMetrics {
	return storage.NewStorageMetrics()
}

// Tests for RetryMiddleware

func TestRetryMiddleware_ImplementsStorageInterface(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)

	// This should compile - middleware must implement storage.Storage interface
	var _ storage.Storage = retryMiddleware
}

func TestRetryMiddleware_Get_SuccessOnFirstAttempt(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	// Test successful get on first attempt
	reader, err := retryMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer func() { _ = reader.Close() }()

	// Verify only one call was made
	assert.Len(t, mockStorage.getCalls, 1)
	assert.Equal(t, "test-key", mockStorage.getCalls[0])
}

func TestRetryMiddleware_Get_RetryOnTransientError(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error (network error)
	networkErr := storage.NewStorageError("get", errors.New("connection refused"), storage.ErrorContext{
		Key: "test-key",
	})
	networkErr.ErrorType = storage.ErrorTypeNetwork
	networkErr.Retryable = true

	// Fail first 2 attempts, succeed on third
	mockStorage.SetFailures(2, networkErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	start := time.Now()
	reader, err := retryMiddleware.Get(ctx, "test-key")
	duration := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, reader)
	defer func() { _ = reader.Close() }()

	// Verify retry occurred (3 calls total)
	assert.Len(t, mockStorage.getCalls, 3)

	// Verify backoff delay occurred (at least initial delay * 2 attempts)
	assert.Greater(t, duration, 20*time.Millisecond)
}

func TestRetryMiddleware_Get_FailOnNonRetryableError(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a non-retryable error (authentication error)
	authErr := storage.NewStorageError("get", errors.New("invalid credentials"), storage.ErrorContext{
		Key: "test-key",
	})
	authErr.ErrorType = storage.ErrorTypeAuthentication
	authErr.Retryable = false

	mockStorage.SetFailures(3, authErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	reader, err := retryMiddleware.Get(ctx, "test-key")

	require.Error(t, err)
	require.Nil(t, reader)

	// Verify only one call was made (no retry for non-retryable errors)
	assert.Len(t, mockStorage.getCalls, 1)
}

func TestRetryMiddleware_Put_RetryOnTransientError(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error
	networkErr := storage.NewStorageError("put", errors.New("timeout"), storage.ErrorContext{
		Key: "test-key",
	})
	networkErr.ErrorType = storage.ErrorTypeTimeout
	networkErr.Retryable = true

	// Fail first attempt, succeed on second
	mockStorage.SetFailures(1, networkErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	err := retryMiddleware.Put(ctx, "test-key", strings.NewReader("test content"))

	require.NoError(t, err)

	// Verify retry occurred (2 calls total)
	assert.Len(t, mockStorage.putCalls, 2)
}

func TestRetryMiddleware_Delete_MaxRetriesExceeded(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error that will persist
	networkErr := storage.NewStorageError("delete", errors.New("service unavailable"), storage.ErrorContext{
		Key: "test-key",
	})
	networkErr.ErrorType = storage.ErrorTypeServiceUnavailable
	networkErr.Retryable = true

	// Fail all attempts
	mockStorage.SetFailures(5, networkErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	err := retryMiddleware.Delete(ctx, "test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")

	// Verify max attempts were made
	assert.Len(t, mockStorage.deleteCalls, 3)
}

func TestRetryMiddleware_List_SuccessAfterRetry(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error
	networkErr := storage.NewStorageError("list", errors.New("connection reset"), storage.ErrorContext{})
	networkErr.ErrorType = storage.ErrorTypeNetwork
	networkErr.Retryable = true

	// Fail first attempt, succeed on second
	mockStorage.SetFailures(1, networkErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	objects, err := retryMiddleware.List(ctx, storage.ListOptions{Prefix: "test/"})

	require.NoError(t, err)
	require.NotNil(t, objects)
	assert.Len(t, objects, 2)
}

func TestRetryMiddleware_Health_RetryOnTransientError(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error
	networkErr := storage.NewStorageError("health", errors.New("connection timeout"), storage.ErrorContext{})
	networkErr.ErrorType = storage.ErrorTypeTimeout
	networkErr.Retryable = true

	// Fail first 2 attempts, succeed on third
	mockStorage.SetFailures(2, networkErr)

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	err := retryMiddleware.Health(ctx)

	require.NoError(t, err)
}

func TestRetryMiddleware_ContextCancellation(t *testing.T) {
	mockStorage := NewMockStorage()

	// Create a retryable error
	networkErr := storage.NewStorageError("get", errors.New("connection refused"), storage.ErrorContext{
		Key: "test-key",
	})
	networkErr.ErrorType = storage.ErrorTypeNetwork
	networkErr.Retryable = true

	// Fail all attempts
	mockStorage.SetFailures(10, networkErr)

	config := &RetryConfig{
		MaxAttempts:       5,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
	}

	retryMiddleware := NewRetryMiddleware(mockStorage, config)

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	reader, err := retryMiddleware.Get(ctx, "test-key")

	require.Error(t, err)
	require.Nil(t, reader)

	// Should have context.DeadlineExceeded error
	assert.True(t, errors.Is(err, context.DeadlineExceeded))

	// Should have made at least 1 attempt but not all 5
	assert.GreaterOrEqual(t, len(mockStorage.getCalls), 1)
	assert.Less(t, len(mockStorage.getCalls), 5)
}

func TestRetryConfig_Defaults(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
	assert.NotNil(t, config.ShouldRetry)
}

func TestRetryConfig_ShouldRetry(t *testing.T) {
	config := DefaultRetryConfig()

	tests := []struct {
		name        string
		err         *storage.StorageError
		attempt     int
		shouldRetry bool
	}{
		{
			name: "network error should retry",
			err: &storage.StorageError{
				ErrorType: storage.ErrorTypeNetwork,
				Retryable: true,
			},
			attempt:     0,
			shouldRetry: true,
		},
		{
			name: "auth error should not retry",
			err: &storage.StorageError{
				ErrorType: storage.ErrorTypeAuthentication,
				Retryable: false,
			},
			attempt:     0,
			shouldRetry: false,
		},
		{
			name: "max attempts exceeded",
			err: &storage.StorageError{
				ErrorType: storage.ErrorTypeNetwork,
				Retryable: true,
			},
			attempt:     3,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldRetry(tt.err, tt.attempt)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

// Benchmark tests
func BenchmarkRetryMiddleware_Get_NoRetry(b *testing.B) {
	mockStorage := NewMockStorage()
	config := DefaultRetryConfig()
	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := retryMiddleware.Get(ctx, fmt.Sprintf("key-%d", i))
		if err == nil && reader != nil {
			_ = reader.Close()
		}
	}
}

func BenchmarkRetryMiddleware_Get_WithRetry(b *testing.B) {
	mockStorage := NewMockStorage()

	// Setup to fail once then succeed
	networkErr := storage.NewStorageError("get", errors.New("connection refused"), storage.ErrorContext{})
	networkErr.ErrorType = storage.ErrorTypeNetwork
	networkErr.Retryable = true

	config := DefaultRetryConfig()
	retryMiddleware := NewRetryMiddleware(mockStorage, config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockStorage.SetFailures(1, networkErr)
		reader, err := retryMiddleware.Get(ctx, fmt.Sprintf("key-%d", i))
		if err == nil && reader != nil {
			_ = reader.Close()
		}
	}
}
