package storage

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageError_Error(t *testing.T) {
	tests := []struct {
		name           string
		storageError   *StorageError
		expectedOutput string
	}{
		{
			name: "error with original error",
			storageError: &StorageError{
				Operation:     "put_object",
				ErrorType:     ErrorTypeNetwork,
				Message:       "Network connectivity issue",
				OriginalError: errors.New("connection refused"),
			},
			expectedOutput: "storage put_object failed (network): Network connectivity issue (original: connection refused)",
		},
		{
			name: "error without original error",
			storageError: &StorageError{
				Operation:     "get_object",
				ErrorType:     ErrorTypeNotFound,
				Message:       "Object not found",
				OriginalError: nil,
			},
			expectedOutput: "storage get_object failed (not_found): Object not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedOutput, tt.storageError.Error())
		})
	}
}

func TestStorageError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	storageErr := &StorageError{
		OriginalError: originalErr,
	}

	unwrapped := storageErr.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
}

func TestStorageError_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		retryable  bool
		expectTrue bool
	}{
		{"retryable error", true, true},
		{"non-retryable error", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageErr := &StorageError{
				Retryable: tt.retryable,
			}

			assert.Equal(t, tt.expectTrue, storageErr.IsRetryable())
		})
	}
}

func TestStorageError_GetRetryDelay(t *testing.T) {
	tests := []struct {
		name          string
		retryAfter    time.Duration
		errorType     ErrorType
		expectedDelay time.Duration
	}{
		{
			name:          "explicit retry after",
			retryAfter:    5 * time.Second,
			errorType:     ErrorTypeNetwork,
			expectedDelay: 5 * time.Second,
		},
		{
			name:          "network error default",
			retryAfter:    0,
			errorType:     ErrorTypeNetwork,
			expectedDelay: 1 * time.Second,
		},
		{
			name:          "timeout error default",
			retryAfter:    0,
			errorType:     ErrorTypeTimeout,
			expectedDelay: 1 * time.Second,
		},
		{
			name:          "service unavailable default",
			retryAfter:    0,
			errorType:     ErrorTypeServiceUnavailable,
			expectedDelay: 5 * time.Second,
		},
		{
			name:          "rate limit default",
			retryAfter:    0,
			errorType:     ErrorTypeRateLimit,
			expectedDelay: 10 * time.Second,
		},
		{
			name:          "other error default",
			retryAfter:    0,
			errorType:     ErrorTypeInternal,
			expectedDelay: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageErr := &StorageError{
				ErrorType:  tt.errorType,
				RetryAfter: tt.retryAfter,
			}

			delay := storageErr.GetRetryDelay()
			assert.Equal(t, tt.expectedDelay, delay)
		})
	}
}

func TestNewStorageError(t *testing.T) {
	originalErr := errors.New("test error")
	context := ErrorContext{
		Bucket:     "test-bucket",
		Key:        "test-key",
		HTTPStatus: http.StatusInternalServerError,
	}

	storageErr := NewStorageError("test_operation", originalErr, context)

	assert.NotNil(t, storageErr)
	assert.Equal(t, "test_operation", storageErr.Operation)
	assert.Equal(t, originalErr, storageErr.OriginalError)
	assert.Equal(t, context, storageErr.Context)
	assert.False(t, storageErr.Timestamp.IsZero())

	// Should classify as internal error due to HTTP 500 status
	assert.Equal(t, ErrorTypeInternal, storageErr.ErrorType)
	assert.True(t, storageErr.Retryable)
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		context            ErrorContext
		expectedErrorType  ErrorType
		expectedRetryable  bool
		expectedRetryAfter time.Duration
	}{
		{
			name:               "nil error",
			err:                nil,
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeUnknown,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "network error",
			err:                errors.New("connection refused"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeNetwork,
			expectedRetryable:  true,
			expectedRetryAfter: 1 * time.Second,
		},
		{
			name:               "timeout error",
			err:                errors.New("timeout occurred"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeTimeout,
			expectedRetryable:  true,
			expectedRetryAfter: 2 * time.Second,
		},
		{
			name:               "HTTP 401 unauthorized",
			err:                errors.New("unauthorized"),
			context:            ErrorContext{HTTPStatus: http.StatusUnauthorized},
			expectedErrorType:  ErrorTypeAuthentication,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "HTTP 403 forbidden",
			err:                errors.New("forbidden"),
			context:            ErrorContext{HTTPStatus: http.StatusForbidden},
			expectedErrorType:  ErrorTypeAuthorization,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "HTTP 404 not found",
			err:                errors.New("not found"),
			context:            ErrorContext{HTTPStatus: http.StatusNotFound},
			expectedErrorType:  ErrorTypeNotFound,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "HTTP 400 bad request",
			err:                errors.New("bad request"),
			context:            ErrorContext{HTTPStatus: http.StatusBadRequest},
			expectedErrorType:  ErrorTypeInvalidRequest,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "HTTP 413 payload too large",
			err:                errors.New("payload too large"),
			context:            ErrorContext{HTTPStatus: http.StatusRequestEntityTooLarge},
			expectedErrorType:  ErrorTypeQuotaExceeded,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "HTTP 507 insufficient storage",
			err:                errors.New("insufficient storage"),
			context:            ErrorContext{HTTPStatus: http.StatusInsufficientStorage},
			expectedErrorType:  ErrorTypeStorageFull,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name: "HTTP 429 rate limit with retry-after",
			err:  errors.New("too many requests"),
			context: ErrorContext{
				HTTPStatus: http.StatusTooManyRequests,
				Headers:    map[string]string{"Retry-After": "15"},
			},
			expectedErrorType:  ErrorTypeRateLimit,
			expectedRetryable:  true,
			expectedRetryAfter: 15 * time.Second,
		},
		{
			name:               "HTTP 500 internal server error",
			err:                errors.New("internal server error"),
			context:            ErrorContext{HTTPStatus: http.StatusInternalServerError},
			expectedErrorType:  ErrorTypeInternal,
			expectedRetryable:  true,
			expectedRetryAfter: 5 * time.Second,
		},
		{
			name:               "HTTP 502 bad gateway",
			err:                errors.New("bad gateway"),
			context:            ErrorContext{HTTPStatus: http.StatusBadGateway},
			expectedErrorType:  ErrorTypeServiceUnavailable,
			expectedRetryable:  true,
			expectedRetryAfter: 5 * time.Second,
		},
		{
			name:               "HTTP 503 service unavailable",
			err:                errors.New("service unavailable"),
			context:            ErrorContext{HTTPStatus: http.StatusServiceUnavailable},
			expectedErrorType:  ErrorTypeServiceUnavailable,
			expectedRetryable:  true,
			expectedRetryAfter: 5 * time.Second,
		},
		{
			name:               "HTTP 504 gateway timeout",
			err:                errors.New("gateway timeout"),
			context:            ErrorContext{HTTPStatus: http.StatusGatewayTimeout},
			expectedErrorType:  ErrorTypeServiceUnavailable,
			expectedRetryable:  true,
			expectedRetryAfter: 5 * time.Second,
		},
		{
			name:               "quota exceeded content",
			err:                errors.New("quota exceeded for user"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeQuotaExceeded,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "disk full content",
			err:                errors.New("no space left on device"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeStorageFull,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "corruption content",
			err:                errors.New("checksum mismatch detected"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeCorruption,
			expectedRetryable:  true,
			expectedRetryAfter: 1 * time.Second,
		},
		{
			name:               "authentication content",
			err:                errors.New("invalid credentials provided"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeAuthentication,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "authorization content",
			err:                errors.New("permission denied"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeAuthorization,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "configuration content",
			err:                errors.New("invalid configuration detected"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeConfiguration,
			expectedRetryable:  false,
			expectedRetryAfter: 0,
		},
		{
			name:               "unknown error",
			err:                errors.New("some random error"),
			context:            ErrorContext{},
			expectedErrorType:  ErrorTypeInternal,
			expectedRetryable:  true,
			expectedRetryAfter: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType, retryable, retryAfter := classifyError(tt.err, tt.context)

			assert.Equal(t, tt.expectedErrorType, errorType)
			assert.Equal(t, tt.expectedRetryable, retryable)
			assert.Equal(t, tt.expectedRetryAfter, retryAfter)
		})
	}
}

// Custom network error for testing
type testNetworkError struct {
	msg       string
	timeout   bool
	temporary bool
}

func (e *testNetworkError) Error() string   { return e.msg }
func (e *testNetworkError) Timeout() bool   { return e.timeout }
func (e *testNetworkError) Temporary() bool { return e.temporary }

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "network error interface - temporary",
			err:      &testNetworkError{msg: "network error", temporary: true},
			expected: true,
		},
		{
			name:     "network error interface - not temporary",
			err:      &testNetworkError{msg: "network error", temporary: false},
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "connection timeout",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "host unreachable",
			err:      errors.New("host unreachable"),
			expected: true,
		},
		{
			name:     "no route to host",
			err:      errors.New("no route to host"),
			expected: true,
		},
		{
			name:     "dns error",
			err:      errors.New("dns resolution failed"),
			expected: true,
		},
		{
			name:     "non-network error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "network timeout error",
			err:      &testNetworkError{msg: "timeout", timeout: true},
			expected: true,
		},
		{
			name:     "network non-timeout error",
			err:      &testNetworkError{msg: "not timeout", timeout: false},
			expected: false,
		},
		{
			name:     "timeout in message",
			err:      errors.New("operation timeout"),
			expected: true,
		},
		{
			name:     "deadline exceeded in message",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "non-timeout error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRetryAfterHeader(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected time.Duration
	}{
		{
			name:     "nil headers",
			headers:  nil,
			expected: 10 * time.Second,
		},
		{
			name:     "empty headers",
			headers:  map[string]string{},
			expected: 10 * time.Second,
		},
		{
			name:     "Retry-After header (capitalized)",
			headers:  map[string]string{"Retry-After": "15"},
			expected: 15 * time.Second,
		},
		{
			name:     "retry-after header (lowercase)",
			headers:  map[string]string{"retry-after": "30"},
			expected: 30 * time.Second,
		},
		{
			name:     "invalid Retry-After header",
			headers:  map[string]string{"Retry-After": "invalid"},
			expected: 10 * time.Second,
		},
		{
			name:     "no retry-after header",
			headers:  map[string]string{"Other-Header": "value"},
			expected: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfterHeader(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateErrorMessage(t *testing.T) {
	tests := []struct {
		name        string
		errorType   ErrorType
		originalErr error
		context     ErrorContext
		expected    string
	}{
		{
			name:        "network error",
			errorType:   ErrorTypeNetwork,
			originalErr: errors.New("connection failed"),
			context:     ErrorContext{},
			expected:    "Network connectivity issue while accessing storage (server may be unreachable)",
		},
		{
			name:        "timeout error",
			errorType:   ErrorTypeTimeout,
			originalErr: errors.New("timeout"),
			context:     ErrorContext{},
			expected:    "Storage operation timed out (server response too slow)",
		},
		{
			name:        "authentication error",
			errorType:   ErrorTypeAuthentication,
			originalErr: errors.New("auth failed"),
			context:     ErrorContext{},
			expected:    "Storage authentication failed (check credentials)",
		},
		{
			name:        "authorization error",
			errorType:   ErrorTypeAuthorization,
			originalErr: errors.New("forbidden"),
			context:     ErrorContext{},
			expected:    "Storage authorization denied (insufficient permissions)",
		},
		{
			name:        "quota exceeded with file size",
			errorType:   ErrorTypeQuotaExceeded,
			originalErr: errors.New("quota exceeded"),
			context:     ErrorContext{FileSize: 1024},
			expected:    "Storage quota exceeded (file size: 1024 bytes)",
		},
		{
			name:        "storage full error",
			errorType:   ErrorTypeStorageFull,
			originalErr: errors.New("disk full"),
			context:     ErrorContext{},
			expected:    "Storage system is full (cannot accept new files)",
		},
		{
			name:        "corruption error",
			errorType:   ErrorTypeCorruption,
			originalErr: errors.New("checksum failed"),
			context:     ErrorContext{},
			expected:    "Data corruption detected during storage operation",
		},
		{
			name:        "rate limit error",
			errorType:   ErrorTypeRateLimit,
			originalErr: errors.New("rate limit"),
			context:     ErrorContext{},
			expected:    "Storage rate limit exceeded (too many requests)",
		},
		{
			name:        "service unavailable error",
			errorType:   ErrorTypeServiceUnavailable,
			originalErr: errors.New("service down"),
			context:     ErrorContext{},
			expected:    "Storage service temporarily unavailable",
		},
		{
			name:        "not found with key",
			errorType:   ErrorTypeNotFound,
			originalErr: errors.New("not found"),
			context:     ErrorContext{Key: "test-key"},
			expected:    "Storage object not found: test-key",
		},
		{
			name:        "invalid request error",
			errorType:   ErrorTypeInvalidRequest,
			originalErr: errors.New("bad request"),
			context:     ErrorContext{},
			expected:    "Invalid storage request (check parameters)",
		},
		{
			name:        "configuration error",
			errorType:   ErrorTypeConfiguration,
			originalErr: errors.New("config error"),
			context:     ErrorContext{},
			expected:    "Storage configuration error (check settings)",
		},
		{
			name:        "internal error",
			errorType:   ErrorTypeInternal,
			originalErr: errors.New("internal error"),
			context:     ErrorContext{},
			expected:    "Internal storage error (server-side issue)",
		},
		{
			name:        "unknown error with original",
			errorType:   ErrorTypeUnknown,
			originalErr: errors.New("mystery error"),
			context:     ErrorContext{},
			expected:    "Storage operation failed: mystery error",
		},
		{
			name:        "unknown error without original",
			errorType:   ErrorTypeUnknown,
			originalErr: nil,
			context:     ErrorContext{},
			expected:    "Unknown storage error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateErrorMessage(tt.errorType, tt.originalErr, tt.context)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
	
	expectedRetryableErrors := []ErrorType{
		ErrorTypeNetwork,
		ErrorTypeTimeout,
		ErrorTypeServiceUnavailable,
		ErrorTypeRateLimit,
		ErrorTypeInternal,
		ErrorTypeCorruption,
	}
	assert.ElementsMatch(t, expectedRetryableErrors, config.RetryableErrors)
}

func TestRetryConfig_ShouldRetry(t *testing.T) {
	config := DefaultRetryConfig()

	tests := []struct {
		name        string
		err         *StorageError
		attempt     int
		shouldRetry bool
	}{
		{
			name: "retryable network error within max attempts",
			err: &StorageError{
				ErrorType: ErrorTypeNetwork,
				Retryable: true,
			},
			attempt:     1,
			shouldRetry: true,
		},
		{
			name: "retryable error exceeding max attempts",
			err: &StorageError{
				ErrorType: ErrorTypeNetwork,
				Retryable: true,
			},
			attempt:     5, // Exceeds MaxAttempts (3)
			shouldRetry: false,
		},
		{
			name: "non-retryable error",
			err: &StorageError{
				ErrorType: ErrorTypeAuthentication,
				Retryable: false,
			},
			attempt:     1,
			shouldRetry: false,
		},
		{
			name: "retryable type not in list",
			err: &StorageError{
				ErrorType: ErrorTypeNotFound, // Not in retryable list
				Retryable: true,
			},
			attempt:     1,
			shouldRetry: false,
		},
		{
			name: "retryable timeout error",
			err: &StorageError{
				ErrorType: ErrorTypeTimeout,
				Retryable: true,
			},
			attempt:     2,
			shouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldRetry(tt.err, tt.attempt)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

func TestRetryConfig_CalculateDelay(t *testing.T) {
	config := DefaultRetryConfig()

	tests := []struct {
		name            string
		attempt         int
		expectedDelay   time.Duration
		maxExpectedDelay time.Duration
	}{
		{
			name:            "attempt 0 or negative",
			attempt:         0,
			expectedDelay:   1 * time.Second, // Initial delay
		},
		{
			name:            "attempt 1",
			attempt:         1,
			expectedDelay:   2 * time.Second, // 1 * 2^1
		},
		{
			name:            "attempt 2",
			attempt:         2,
			expectedDelay:   4 * time.Second, // 1 * 2^2
		},
		{
			name:             "attempt causing max delay",
			attempt:          10,
			maxExpectedDelay: 30 * time.Second, // Should be capped at MaxDelay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := config.CalculateDelay(tt.attempt)
			
			if tt.maxExpectedDelay > 0 {
				assert.LessOrEqual(t, delay, tt.maxExpectedDelay)
			} else {
				assert.Equal(t, tt.expectedDelay, delay)
			}
		})
	}
}

// Benchmark tests for error handling performance

func BenchmarkNewStorageError(b *testing.B) {
	originalErr := errors.New("test error")
	context := ErrorContext{
		Bucket:     "test-bucket",
		Key:        "test-key",
		HTTPStatus: http.StatusInternalServerError,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewStorageError("test_operation", originalErr, context)
	}
}

func BenchmarkClassifyError(b *testing.B) {
	err := errors.New("connection refused")
	context := ErrorContext{HTTPStatus: http.StatusInternalServerError}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = classifyError(err, context)
	}
}

func BenchmarkIsNetworkError(b *testing.B) {
	err := errors.New("connection refused")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isNetworkError(err)
	}
}

func BenchmarkRetryConfig_ShouldRetry(b *testing.B) {
	config := DefaultRetryConfig()
	err := &StorageError{
		ErrorType: ErrorTypeNetwork,
		Retryable: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.ShouldRetry(err, 1)
	}
}