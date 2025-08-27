package errors

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorType_Classification(t *testing.T) {
	tests := []struct {
		name           string
		errorType      ErrorType
		expectedCode   int
		expectedSeverity Severity
	}{
		{
			name:           "validation error",
			errorType:      ErrorTypeValidation,
			expectedCode:   400,
			expectedSeverity: SeverityWarning,
		},
		{
			name:           "authentication error", 
			errorType:      ErrorTypeAuth,
			expectedCode:   401,
			expectedSeverity: SeverityError,
		},
		{
			name:           "authorization error",
			errorType:      ErrorTypeAuthorization,
			expectedCode:   403,
			expectedSeverity: SeverityError,
		},
		{
			name:           "not found error",
			errorType:      ErrorTypeNotFound,
			expectedCode:   404,
			expectedSeverity: SeverityWarning,
		},
		{
			name:           "rate limit error",
			errorType:      ErrorTypeRateLimit,
			expectedCode:   429,
			expectedSeverity: SeverityWarning,
		},
		{
			name:           "execution error",
			errorType:      ErrorTypeExecution,
			expectedCode:   500,
			expectedSeverity: SeverityError,
		},
		{
			name:           "resource error",
			errorType:      ErrorTypeResource,
			expectedCode:   507,
			expectedSeverity: SeverityCritical,
		},
		{
			name:           "security error",
			errorType:      ErrorTypeSecurity,
			expectedCode:   403,
			expectedSeverity: SeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError(tt.errorType, "test error", nil)
			
			assert.Equal(t, tt.expectedCode, err.HTTPStatus())
			assert.Equal(t, tt.expectedSeverity, err.Severity())
			assert.Equal(t, tt.errorType, err.Type())
		})
	}
}

func TestCHTTPError_Creation(t *testing.T) {
	originalErr := errors.New("original error")
	
	err := NewError(ErrorTypeValidation, "validation failed", originalErr).
		WithField("field", "email").
		WithField("value", "invalid@").
		WithCorrelationID("req-123")
	
	assert.Equal(t, "validation failed", err.Message())
	assert.Equal(t, originalErr, err.Unwrap())
	assert.Equal(t, "invalid@", err.Field("value"))
	assert.Equal(t, "req-123", err.CorrelationID())
	assert.True(t, err.HasField("field"))
}

func TestCHTTPError_Wrapping(t *testing.T) {
	originalErr := errors.New("database connection failed")
	
	wrapped := WrapError(ErrorTypeResource, "failed to save data", originalErr).
		WithContext("operation", "user_save").
		WithField("user_id", "123")
	
	// Test error unwrapping
	assert.True(t, errors.Is(wrapped, originalErr))
	assert.Equal(t, "failed to save data", wrapped.Message())
	assert.Equal(t, "user_save", wrapped.Context("operation"))
}

func TestErrorBuilder_FluentAPI(t *testing.T) {
	err := NewError(ErrorTypeExecution, "command failed", nil).
		WithField("command", "pylint").
		WithField("exit_code", 2).
		WithContext("working_dir", "/tmp/analysis").
		WithCorrelationID("analysis-456").
		WithRetryable(true).
		WithTimeout(30 * time.Second)
	
	assert.Equal(t, "command failed", err.Message())
	assert.Equal(t, "pylint", err.Field("command"))
	assert.Equal(t, 2, err.Field("exit_code"))
	assert.Equal(t, "/tmp/analysis", err.Context("working_dir"))
	assert.Equal(t, "analysis-456", err.CorrelationID())
	assert.True(t, err.IsRetryable())
	assert.Equal(t, 30*time.Second, err.Timeout())
}

func TestErrorMiddleware_HandlesErrors(t *testing.T) {
	app := fiber.New()
	
	// Setup error middleware
	middleware := NewErrorMiddleware(ErrorMiddlewareConfig{
		EnableStackTrace: true,
		EnableLogging:    true,
		LogLevel:        "error",
	})
	
	app.Use(middleware)
	
	// Test route that returns a CHTTP error
	app.Get("/validation-error", func(c *fiber.Ctx) error {
		return NewError(ErrorTypeValidation, "invalid input", nil).
			WithField("field", "email")
	})
	
	app.Get("/execution-error", func(c *fiber.Ctx) error {
		return NewError(ErrorTypeExecution, "command failed", nil).
			WithCorrelationID("test-123")
	})
	
	app.Get("/panic", func(c *fiber.Ctx) error {
		panic("something went wrong")
	})
	
	// Test validation error response
	req, _ := http.NewRequest(http.MethodGet, "/validation-error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	
	// Test execution error response
	req, _ = http.NewRequest(http.MethodGet, "/execution-error", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	
	// Test panic recovery
	req, _ = http.NewRequest(http.MethodGet, "/panic", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestErrorResponse_JSONStructure(t *testing.T) {
	err := NewError(ErrorTypeValidation, "validation failed", nil).
		WithField("field", "email").
		WithField("value", "invalid").
		WithCorrelationID("req-789")
	
	response := err.ToResponse()
	
	assert.Equal(t, "error", response.Status)
	assert.Equal(t, "validation", response.Type)
	assert.Equal(t, "validation failed", response.Message)
	assert.Equal(t, "req-789", response.CorrelationID)
	assert.NotEmpty(t, response.Timestamp)
	assert.Contains(t, response.Details, "field")
	assert.Contains(t, response.Details, "value")
}

func TestErrorMetrics_Tracking(t *testing.T) {
	metrics := NewErrorMetrics()
	
	// Record various error types
	metrics.RecordError(ErrorTypeValidation, "warning")
	metrics.RecordError(ErrorTypeValidation, "warning") 
	metrics.RecordError(ErrorTypeExecution, "error")
	metrics.RecordError(ErrorTypeResource, "critical")
	
	stats := metrics.GetStats()
	
	assert.Equal(t, 4, stats.TotalErrors)
	assert.Equal(t, 2, stats.ErrorsByType[ErrorTypeValidation])
	assert.Equal(t, 1, stats.ErrorsByType[ErrorTypeExecution])
	assert.Equal(t, 1, stats.ErrorsByType[ErrorTypeResource])
	assert.Equal(t, 2, stats.ErrorsBySeverity["warning"])
	assert.Equal(t, 1, stats.ErrorsBySeverity["error"])
	assert.Equal(t, 1, stats.ErrorsBySeverity["critical"])
}

func TestCircuitBreaker_ErrorHandling(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		TimeoutDuration:  100 * time.Millisecond,
		ResetTimeout:     500 * time.Millisecond,
	})
	
	// Function that always fails
	failingFunc := func() error {
		return NewError(ErrorTypeExecution, "operation failed", nil)
	}
	
	// Should allow calls initially
	assert.Equal(t, CircuitBreakerStateClosed, cb.State())
	
	// Fail enough times to open circuit
	for i := 0; i < 3; i++ {
		err := cb.Call(failingFunc)
		assert.Error(t, err)
	}
	
	// Circuit should now be open
	assert.Equal(t, CircuitBreakerStateOpen, cb.State())
	
	// Should fail fast now
	err := cb.Call(failingFunc)
	assert.Error(t, err)
	assert.True(t, IsCircuitBreakerOpen(err))
}

func TestErrorRecovery_GracefulDegradation(t *testing.T) {
	recovery := NewErrorRecovery(ErrorRecoveryConfig{
		EnableFallback:     true,
		FallbackTimeout:    time.Second,
		RetryAttempts:      3,
		RetryDelay:        100 * time.Millisecond,
	})
	
	attempts := 0
	
	// Function that succeeds on 3rd attempt
	operation := func() error {
		attempts++
		if attempts < 3 {
			return NewError(ErrorTypeExecution, "temporary failure", nil).
				WithRetryable(true)
		}
		return nil
	}
	
	err := recovery.ExecuteWithRetry(context.Background(), operation)
	
	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestHealthCheck_ErrorIntegration(t *testing.T) {
	health := NewHealthChecker()
	
	// Add some components
	health.AddComponent("database", func() error {
		return nil // healthy
	})
	
	health.AddComponent("external_api", func() error {
		return NewError(ErrorTypeResource, "API unavailable", nil)
	})
	
	status := health.CheckHealth()
	
	assert.Equal(t, "degraded", status.Status)
	assert.Equal(t, "healthy", status.Components["database"].Status)
	assert.Equal(t, "unhealthy", status.Components["external_api"].Status)
	assert.Contains(t, status.Components["external_api"].Error, "API unavailable")
}

func TestErrorContext_Propagation(t *testing.T) {
	ctx := context.Background()
	
	// Add error context
	ctx = WithErrorContext(ctx, "request_id", "req-123")
	ctx = WithErrorContext(ctx, "user_id", "user-456") 
	ctx = WithErrorContext(ctx, "operation", "file_upload")
	
	// Create error with context
	err := NewErrorWithContext(ctx, ErrorTypeValidation, "invalid file", nil)
	
	assert.Equal(t, "req-123", err.Context("request_id"))
	assert.Equal(t, "user-456", err.Context("user_id"))
	assert.Equal(t, "file_upload", err.Context("operation"))
}

func TestErrorFiltering_SensitiveData(t *testing.T) {
	filter := NewErrorFilter(ErrorFilterConfig{
		SensitiveFields: []string{"password", "token", "secret"},
		MaskChar:        "*",
	})
	
	err := NewError(ErrorTypeAuth, "authentication failed", nil).
		WithField("username", "john").
		WithField("password", "secret123").
		WithField("token", "jwt-token-here")
	
	filtered := filter.FilterError(err)
	
	assert.Equal(t, "john", filtered.Field("username"))
	assert.Equal(t, "***", filtered.Field("password"))
	assert.Equal(t, "***", filtered.Field("token"))
}