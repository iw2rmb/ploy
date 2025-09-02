package storage

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// StorageError represents a comprehensive storage operation error
type StorageError struct {
	Operation     string        `json:"operation"`
	ErrorType     ErrorType     `json:"error_type"`
	Message       string        `json:"message"`
	OriginalError error         `json:"original_error,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
	Retryable     bool          `json:"retryable"`
	RetryAfter    time.Duration `json:"retry_after,omitempty"`
	Context       ErrorContext  `json:"context,omitempty"`
}

// ErrorType categorizes different types of storage errors
type ErrorType string

const (
	ErrorTypeNetwork         ErrorType = "network"
	ErrorTypeAuthentication  ErrorType = "authentication"
	ErrorTypeAuthorization   ErrorType = "authorization"
	ErrorTypeQuotaExceeded   ErrorType = "quota_exceeded"
	ErrorTypeStorageFull     ErrorType = "storage_full"
	ErrorTypeCorruption      ErrorType = "corruption"
	ErrorTypeTimeout         ErrorType = "timeout"
	ErrorTypeConfiguration   ErrorType = "configuration"
	ErrorTypeServiceUnavailable ErrorType = "service_unavailable"
	ErrorTypeRateLimit       ErrorType = "rate_limit"
	ErrorTypeInvalidRequest  ErrorType = "invalid_request"
	ErrorTypeNotFound        ErrorType = "not_found"
	ErrorTypeInternal        ErrorType = "internal"
	ErrorTypeUnknown         ErrorType = "unknown"
)

// ErrorContext provides additional context for storage errors
type ErrorContext struct {
	Bucket        string            `json:"bucket,omitempty"`
	Key           string            `json:"key,omitempty"`
	FileSize      int64             `json:"file_size,omitempty"`
	ContentType   string            `json:"content_type,omitempty"`
	HTTPStatus    int               `json:"http_status,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	ServerInfo    string            `json:"server_info,omitempty"`
	AttemptNumber int               `json:"attempt_number,omitempty"`
}

// Error implements the error interface
func (e *StorageError) Error() string {
	if e.OriginalError != nil {
		return fmt.Sprintf("storage %s failed (%s): %s (original: %v)", 
			e.Operation, e.ErrorType, e.Message, e.OriginalError)
	}
	return fmt.Sprintf("storage %s failed (%s): %s", e.Operation, e.ErrorType, e.Message)
}

// Unwrap returns the original error for error inspection
func (e *StorageError) Unwrap() error {
	return e.OriginalError
}

// IsRetryable returns whether this error should be retried
func (e *StorageError) IsRetryable() bool {
	return e.Retryable
}

// GetRetryDelay returns the recommended delay before retry
func (e *StorageError) GetRetryDelay() time.Duration {
	if e.RetryAfter > 0 {
		return e.RetryAfter
	}
	
	// Default retry delays based on error type
	switch e.ErrorType {
	case ErrorTypeNetwork, ErrorTypeTimeout:
		return 1 * time.Second
	case ErrorTypeServiceUnavailable:
		return 5 * time.Second
	case ErrorTypeRateLimit:
		return 10 * time.Second
	default:
		return 2 * time.Second
	}
}

// NewStorageError creates a new storage error with automatic error classification
func NewStorageError(operation string, originalErr error, context ErrorContext) *StorageError {
	errorType, retryable, retryAfter := classifyError(originalErr, context)
	
	return &StorageError{
		Operation:     operation,
		ErrorType:     errorType,
		Message:       generateErrorMessage(errorType, originalErr, context),
		OriginalError: originalErr,
		Timestamp:     time.Now(),
		Retryable:     retryable,
		RetryAfter:    retryAfter,
		Context:       context,
	}
}

// classifyError automatically categorizes errors and determines retry behavior
func classifyError(err error, context ErrorContext) (ErrorType, bool, time.Duration) {
	if err == nil {
		return ErrorTypeUnknown, false, 0
	}

	errStr := strings.ToLower(err.Error())
	
	// HTTP status code classification (check first for precise mapping)
	switch context.HTTPStatus {
	case http.StatusUnauthorized:
		return ErrorTypeAuthentication, false, 0
	case http.StatusForbidden:
		return ErrorTypeAuthorization, false, 0
	case http.StatusNotFound:
		return ErrorTypeNotFound, false, 0
	case http.StatusBadRequest:
		return ErrorTypeInvalidRequest, false, 0
	case http.StatusRequestEntityTooLarge:
		return ErrorTypeQuotaExceeded, false, 0
	case http.StatusInsufficientStorage:
		return ErrorTypeStorageFull, false, 0
	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfterHeader(context.Headers)
		return ErrorTypeRateLimit, true, retryAfter
	case http.StatusInternalServerError:
		return ErrorTypeInternal, true, 5 * time.Second
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrorTypeServiceUnavailable, true, 5 * time.Second
	}
	
	// Network-related errors (check after HTTP status)
	if isNetworkError(err) {
		return ErrorTypeNetwork, true, 1 * time.Second
	}
	
	// Timeout errors (check after HTTP status to avoid override)
	if isTimeoutError(err) {
		return ErrorTypeTimeout, true, 2 * time.Second
	}
	
	// Content-based error classification
	if strings.Contains(errStr, "quota") || strings.Contains(errStr, "limit exceeded") {
		return ErrorTypeQuotaExceeded, false, 0
	}
	
	if strings.Contains(errStr, "no space left") || strings.Contains(errStr, "disk full") {
		return ErrorTypeStorageFull, false, 0
	}
	
	if strings.Contains(errStr, "checksum") || strings.Contains(errStr, "corruption") || 
	   strings.Contains(errStr, "integrity") {
		return ErrorTypeCorruption, true, 1 * time.Second
	}
	
	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "credentials") {
		return ErrorTypeAuthentication, false, 0
	}
	
	if strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "permission") {
		return ErrorTypeAuthorization, false, 0
	}
	
	if strings.Contains(errStr, "configuration") || strings.Contains(errStr, "config") {
		return ErrorTypeConfiguration, false, 0
	}
	
	// Default to retryable internal error
	return ErrorTypeInternal, true, 2 * time.Second
}

// isNetworkError checks if an error is network-related
func isNetworkError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary()
	}
	
	// Check for common network error messages
	errStr := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"network unreachable",
		"host unreachable",
		"no route to host",
		"dns",
	}
	
	for _, pattern := range networkPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// isTimeoutError checks if an error is timeout-related
func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded")
}

// parseRetryAfterHeader extracts retry delay from HTTP headers
func parseRetryAfterHeader(headers map[string]string) time.Duration {
	if headers == nil {
		return 10 * time.Second
	}
	
	retryAfter, exists := headers["Retry-After"]
	if !exists {
		retryAfter, exists = headers["retry-after"]
	}
	
	if exists {
		if duration, err := time.ParseDuration(retryAfter + "s"); err == nil {
			return duration
		}
	}
	
	return 10 * time.Second // Default retry delay for rate limiting
}

// generateErrorMessage creates a user-friendly error message
func generateErrorMessage(errorType ErrorType, originalErr error, context ErrorContext) string {
	switch errorType {
	case ErrorTypeNetwork:
		return fmt.Sprintf("Network connectivity issue while accessing storage (server may be unreachable)")
	case ErrorTypeTimeout:
		return fmt.Sprintf("Storage operation timed out (server response too slow)")
	case ErrorTypeAuthentication:
		return fmt.Sprintf("Storage authentication failed (check credentials)")
	case ErrorTypeAuthorization:
		return fmt.Sprintf("Storage authorization denied (insufficient permissions)")
	case ErrorTypeQuotaExceeded:
		return fmt.Sprintf("Storage quota exceeded (file size: %d bytes)", context.FileSize)
	case ErrorTypeStorageFull:
		return fmt.Sprintf("Storage system is full (cannot accept new files)")
	case ErrorTypeCorruption:
		return fmt.Sprintf("Data corruption detected during storage operation")
	case ErrorTypeRateLimit:
		return fmt.Sprintf("Storage rate limit exceeded (too many requests)")
	case ErrorTypeServiceUnavailable:
		return fmt.Sprintf("Storage service temporarily unavailable")
	case ErrorTypeNotFound:
		return fmt.Sprintf("Storage object not found: %s", context.Key)
	case ErrorTypeInvalidRequest:
		return fmt.Sprintf("Invalid storage request (check parameters)")
	case ErrorTypeConfiguration:
		return fmt.Sprintf("Storage configuration error (check settings)")
	case ErrorTypeInternal:
		return fmt.Sprintf("Internal storage error (server-side issue)")
	default:
		if originalErr != nil {
			return fmt.Sprintf("Storage operation failed: %v", originalErr)
		}
		return "Unknown storage error occurred"
	}
}

// RetryConfig defines retry behavior for storage operations
type RetryConfig struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffMultiplier float64     `json:"backoff_multiplier"`
	RetryableErrors   []ErrorType  `json:"retryable_errors"`
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableErrors: []ErrorType{
			ErrorTypeNetwork,
			ErrorTypeTimeout,
			ErrorTypeServiceUnavailable,
			ErrorTypeRateLimit,
			ErrorTypeInternal,
			ErrorTypeCorruption,
		},
	}
}

// ShouldRetry determines if an error should be retried based on configuration
func (rc *RetryConfig) ShouldRetry(err *StorageError, attempt int) bool {
	if attempt >= rc.MaxAttempts {
		return false
	}
	
	if !err.IsRetryable() {
		return false
	}
	
	// Check if error type is in retryable list
	for _, retryableType := range rc.RetryableErrors {
		if err.ErrorType == retryableType {
			return true
		}
	}
	
	return false
}

// CalculateDelay calculates the delay for a retry attempt using exponential backoff
func (rc *RetryConfig) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return rc.InitialDelay
	}
	
	delay := time.Duration(float64(rc.InitialDelay) * 
		(rc.BackoffMultiplier * float64(attempt)))
	
	if delay > rc.MaxDelay {
		delay = rc.MaxDelay
	}
	
	return delay
}