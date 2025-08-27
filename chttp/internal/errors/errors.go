package errors

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrorType represents the category of error
type ErrorType string

const (
	ErrorTypeValidation     ErrorType = "validation"
	ErrorTypeAuth          ErrorType = "authentication"
	ErrorTypeAuthorization ErrorType = "authorization"
	ErrorTypeNotFound      ErrorType = "not_found"
	ErrorTypeRateLimit     ErrorType = "rate_limit"
	ErrorTypeExecution     ErrorType = "execution"
	ErrorTypeResource      ErrorType = "resource"
	ErrorTypeSecurity      ErrorType = "security"
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeTimeout       ErrorType = "timeout"
	ErrorTypeInternal      ErrorType = "internal"
)

// Severity represents the severity level of an error
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// CHTTPError represents a structured error in the CHTTP system
type CHTTPError struct {
	errorType     ErrorType
	message       string
	originalError error
	fields        map[string]interface{}
	context       map[string]interface{}
	correlationID string
	timestamp     time.Time
	stackTrace    []string
	retryable     bool
	timeout       time.Duration
}

// NewError creates a new CHTTPError
func NewError(errorType ErrorType, message string, originalError error) *CHTTPError {
	return &CHTTPError{
		errorType:     errorType,
		message:       message,
		originalError: originalError,
		fields:        make(map[string]interface{}),
		context:       make(map[string]interface{}),
		timestamp:     time.Now().UTC(),
		stackTrace:    captureStackTrace(),
	}
}

// WrapError wraps an existing error with CHTTPError
func WrapError(errorType ErrorType, message string, originalError error) *CHTTPError {
	return NewError(errorType, message, originalError)
}

// NewErrorWithContext creates a new error with context from the provided context.Context
func NewErrorWithContext(ctx context.Context, errorType ErrorType, message string, originalError error) *CHTTPError {
	err := NewError(errorType, message, originalError)
	
	// Extract context values
	if ctx != nil {
		if reqID := getContextValue(ctx, "request_id"); reqID != "" {
			err.context["request_id"] = reqID
		}
		if userID := getContextValue(ctx, "user_id"); userID != "" {
			err.context["user_id"] = userID
		}
		if operation := getContextValue(ctx, "operation"); operation != "" {
			err.context["operation"] = operation
		}
	}
	
	return err
}

// Error implements the error interface
func (e *CHTTPError) Error() string {
	if e.originalError != nil {
		return fmt.Sprintf("%s: %s", e.message, e.originalError.Error())
	}
	return e.message
}

// Unwrap implements the unwrappable error interface
func (e *CHTTPError) Unwrap() error {
	return e.originalError
}

// Fluent API methods for building errors
func (e *CHTTPError) WithField(key string, value interface{}) *CHTTPError {
	e.fields[key] = value
	return e
}

func (e *CHTTPError) WithContext(key string, value interface{}) *CHTTPError {
	e.context[key] = value
	return e
}

func (e *CHTTPError) WithCorrelationID(id string) *CHTTPError {
	e.correlationID = id
	return e
}

func (e *CHTTPError) WithRetryable(retryable bool) *CHTTPError {
	e.retryable = retryable
	return e
}

func (e *CHTTPError) WithTimeout(timeout time.Duration) *CHTTPError {
	e.timeout = timeout
	return e
}

// Getter methods
func (e *CHTTPError) Type() ErrorType {
	return e.errorType
}

func (e *CHTTPError) Message() string {
	return e.message
}

func (e *CHTTPError) Field(key string) interface{} {
	return e.fields[key]
}

func (e *CHTTPError) HasField(key string) bool {
	_, exists := e.fields[key]
	return exists
}

func (e *CHTTPError) Context(key string) interface{} {
	return e.context[key]
}

func (e *CHTTPError) CorrelationID() string {
	return e.correlationID
}

func (e *CHTTPError) IsRetryable() bool {
	return e.retryable
}

func (e *CHTTPError) Timeout() time.Duration {
	return e.timeout
}

// HTTPStatus returns the appropriate HTTP status code
func (e *CHTTPError) HTTPStatus() int {
	switch e.errorType {
	case ErrorTypeValidation:
		return 400
	case ErrorTypeAuth:
		return 401
	case ErrorTypeAuthorization, ErrorTypeSecurity:
		return 403
	case ErrorTypeNotFound:
		return 404
	case ErrorTypeTimeout:
		return 408
	case ErrorTypeRateLimit:
		return 429
	case ErrorTypeResource:
		return 507
	default:
		return 500
	}
}

// Severity returns the severity level
func (e *CHTTPError) Severity() Severity {
	switch e.errorType {
	case ErrorTypeValidation, ErrorTypeNotFound, ErrorTypeRateLimit:
		return SeverityWarning
	case ErrorTypeAuth, ErrorTypeAuthorization, ErrorTypeExecution, ErrorTypeNetwork, ErrorTypeTimeout:
		return SeverityError
	case ErrorTypeResource, ErrorTypeSecurity:
		return SeverityCritical
	default:
		return SeverityError
	}
}

// ErrorResponse represents the JSON response structure for errors
type ErrorResponse struct {
	Status        string                 `json:"status"`
	Type          string                 `json:"type"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Timestamp     string                 `json:"timestamp"`
	Retryable     bool                   `json:"retryable,omitempty"`
	StackTrace    []string               `json:"stack_trace,omitempty"`
}

// ToResponse converts the error to a response structure
func (e *CHTTPError) ToResponse() *ErrorResponse {
	return &ErrorResponse{
		Status:        "error",
		Type:          string(e.errorType),
		Message:       e.message,
		Details:       e.fields,
		CorrelationID: e.correlationID,
		Timestamp:     e.timestamp.Format(time.RFC3339),
		Retryable:     e.retryable,
		StackTrace:    e.stackTrace,
	}
}

// Context helper functions
func WithErrorContext(ctx context.Context, key string, value interface{}) context.Context {
	return context.WithValue(ctx, key, value)
}

func getContextValue(ctx context.Context, key string) string {
	if value := ctx.Value(key); value != nil {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// captureStackTrace captures the current stack trace
func captureStackTrace() []string {
	var traces []string
	for i := 2; i < 10; i++ { // Skip this function and NewError
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		
		// Simplify file path
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			file = file[idx+1:]
		}
		
		traces = append(traces, fmt.Sprintf("%s:%d", file, line))
	}
	return traces
}

// ErrorMetrics tracks error statistics
type ErrorMetrics struct {
	mu               sync.RWMutex
	totalErrors      int
	errorsByType     map[ErrorType]int
	errorsBySeverity map[string]int
}

// NewErrorMetrics creates a new error metrics tracker
func NewErrorMetrics() *ErrorMetrics {
	return &ErrorMetrics{
		errorsByType:     make(map[ErrorType]int),
		errorsBySeverity: make(map[string]int),
	}
}

// RecordError records an error occurrence
func (m *ErrorMetrics) RecordError(errorType ErrorType, severity string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.totalErrors++
	m.errorsByType[errorType]++
	m.errorsBySeverity[severity]++
}

// ErrorStats represents error statistics
type ErrorStats struct {
	TotalErrors      int                `json:"total_errors"`
	ErrorsByType     map[ErrorType]int  `json:"errors_by_type"`
	ErrorsBySeverity map[string]int     `json:"errors_by_severity"`
}

// GetStats returns current error statistics
func (m *ErrorMetrics) GetStats() ErrorStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Copy maps to avoid race conditions
	typesCopy := make(map[ErrorType]int)
	severityCopy := make(map[string]int)
	
	for k, v := range m.errorsByType {
		typesCopy[k] = v
	}
	
	for k, v := range m.errorsBySeverity {
		severityCopy[k] = v
	}
	
	return ErrorStats{
		TotalErrors:      m.totalErrors,
		ErrorsByType:     typesCopy,
		ErrorsBySeverity: severityCopy,
	}
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState string

const (
	CircuitBreakerStateClosed   CircuitBreakerState = "closed"
	CircuitBreakerStateOpen     CircuitBreakerState = "open"
	CircuitBreakerStateHalfOpen CircuitBreakerState = "half_open"
)

// CircuitBreakerConfig configures a circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int
	TimeoutDuration  time.Duration
	ResetTimeout     time.Duration
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu              sync.RWMutex
	config          CircuitBreakerConfig
	state           CircuitBreakerState
	failures        int
	lastFailureTime time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  CircuitBreakerStateClosed,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	// Check if circuit should be reset
	if cb.state == CircuitBreakerStateOpen && 
		time.Since(cb.lastFailureTime) > cb.config.ResetTimeout {
		cb.state = CircuitBreakerStateHalfOpen
	}
	
	// Fail fast if circuit is open
	if cb.state == CircuitBreakerStateOpen {
		return NewError(ErrorTypeResource, "circuit breaker is open", nil).
			WithField("state", "open")
	}
	
	// Execute function
	err := fn()
	
	if err != nil {
		cb.failures++
		cb.lastFailureTime = time.Now()
		
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitBreakerStateOpen
		}
		
		return err
	}
	
	// Success - reset circuit
	cb.failures = 0
	cb.state = CircuitBreakerStateClosed
	return nil
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsCircuitBreakerOpen checks if an error is due to circuit breaker being open
func IsCircuitBreakerOpen(err error) bool {
	if cErr, ok := err.(*CHTTPError); ok {
		if state, exists := cErr.fields["state"]; exists {
			return state == "open"
		}
	}
	return false
}