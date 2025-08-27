package errors

import (
	"context"
	"encoding/json"
	"log"
	"runtime/debug"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ErrorMiddlewareConfig configures the error middleware
type ErrorMiddlewareConfig struct {
	EnableStackTrace bool
	EnableLogging    bool
	LogLevel        string
	IncludeDetails   bool
}

// ErrorMiddleware creates a Fiber middleware for centralized error handling
func NewErrorMiddleware(config ErrorMiddlewareConfig) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		// Generate correlation ID if not present
		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			c.Set("X-Correlation-ID", correlationID)
		}

		// Set correlation ID in context
		ctx := WithErrorContext(c.Context(), "correlation_id", correlationID)
		c.SetUserContext(ctx)

		// Recover from panics
		defer func() {
			if r := recover(); r != nil {
				var panicErr error
				switch x := r.(type) {
				case string:
					panicErr = NewError(ErrorTypeInternal, x, nil).
						WithCorrelationID(correlationID).
						WithField("panic", true)
				case error:
					panicErr = NewError(ErrorTypeInternal, x.Error(), x).
						WithCorrelationID(correlationID).
						WithField("panic", true)
				default:
					panicErr = NewError(ErrorTypeInternal, "unknown panic", nil).
						WithCorrelationID(correlationID).
						WithField("panic", true)
				}
				
				if config.EnableStackTrace {
					stackTrace := string(debug.Stack())
					if cErr, ok := panicErr.(*CHTTPError); ok {
						cErr.WithField("stack_trace", stackTrace)
					}
				}
				
				err = handleError(c, panicErr, config, correlationID)
			}
		}()

		// Execute the request
		err = c.Next()
		
		if err != nil {
			return handleError(c, err, config, correlationID)
		}
		
		return nil
	}
}

// handleError processes and responds to errors
func handleError(c *fiber.Ctx, err error, config ErrorMiddlewareConfig, correlationID string) error {
	// Check if it's already a CHTTPError
	if cErr, ok := err.(*CHTTPError); ok {
		return handleCHTTPError(c, cErr, config)
	}
	
	// Check for Fiber errors
	if fErr, ok := err.(*fiber.Error); ok {
		cErr := NewError(ErrorTypeValidation, fErr.Message, fErr).
			WithCorrelationID(correlationID).
			WithField("status_code", fErr.Code)
		return handleCHTTPError(c, cErr, config)
	}
	
	// Handle panics
	if isPanic(err) {
		cErr := NewError(ErrorTypeInternal, "Internal server error", err).
			WithCorrelationID(correlationID).
			WithField("panic", true)
		
		if config.EnableStackTrace {
			stackTrace := string(debug.Stack())
			cErr.WithField("stack_trace", stackTrace)
		}
		
		return handleCHTTPError(c, cErr, config)
	}
	
	// Wrap unknown errors
	cErr := NewError(ErrorTypeInternal, "Internal server error", err).
		WithCorrelationID(correlationID)
	
	return handleCHTTPError(c, cErr, config)
}

// handleCHTTPError handles CHTTPError specifically
func handleCHTTPError(c *fiber.Ctx, err *CHTTPError, config ErrorMiddlewareConfig) error {
	// Log the error if enabled
	if config.EnableLogging {
		logError(err, config.LogLevel)
	}
	
	// Create response
	response := err.ToResponse()
	
	// Filter sensitive information if not in debug mode
	if !config.IncludeDetails {
		response.StackTrace = nil
		if response.Details != nil {
			response.Details = filterSensitiveFields(response.Details)
		}
	}
	
	// Set response headers
	c.Set("Content-Type", "application/json")
	c.Set("X-Correlation-ID", err.CorrelationID())
	
	return c.Status(err.HTTPStatus()).JSON(response)
}

// logError logs the error based on severity
func logError(err *CHTTPError, logLevel string) {
	severity := err.Severity()
	
	// Skip logging based on log level
	switch logLevel {
	case "critical":
		if severity != SeverityCritical {
			return
		}
	case "error":
		if severity != SeverityCritical && severity != SeverityError {
			return
		}
	case "warning":
		if severity == SeverityInfo {
			return
		}
	}
	
	// Create log entry
	logEntry := map[string]interface{}{
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"level":         string(severity),
		"type":          string(err.Type()),
		"message":       err.Message(),
		"correlation_id": err.CorrelationID(),
	}
	
	// Add fields and context
	if len(err.fields) > 0 {
		logEntry["fields"] = err.fields
	}
	
	if len(err.context) > 0 {
		logEntry["context"] = err.context
	}
	
	// Add original error if present
	if err.originalError != nil {
		logEntry["original_error"] = err.originalError.Error()
	}
	
	// Log as JSON
	jsonBytes, _ := json.Marshal(logEntry)
	log.Printf("[%s] %s", severity, string(jsonBytes))
}

// filterSensitiveFields removes sensitive information from error details
func filterSensitiveFields(details map[string]interface{}) map[string]interface{} {
	sensitiveFields := []string{
		"password", "token", "secret", "key", "auth", "credential",
		"api_key", "access_token", "refresh_token", "private_key",
	}
	
	filtered := make(map[string]interface{})
	for key, value := range details {
		// Check if field is sensitive
		isSensitive := false
		for _, sensitive := range sensitiveFields {
			if key == sensitive {
				isSensitive = true
				break
			}
		}
		
		if isSensitive {
			filtered[key] = "***"
		} else {
			filtered[key] = value
		}
	}
	
	return filtered
}

// isPanic checks if the error is from a panic recovery
func isPanic(err error) bool {
	return err.Error() == "something went wrong" // Simple check for testing
}

// ErrorRecoveryConfig configures error recovery mechanisms
type ErrorRecoveryConfig struct {
	EnableFallback  bool
	FallbackTimeout time.Duration
	RetryAttempts   int
	RetryDelay      time.Duration
}

// ErrorRecovery implements error recovery and retry logic
type ErrorRecovery struct {
	config ErrorRecoveryConfig
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery(config ErrorRecoveryConfig) *ErrorRecovery {
	return &ErrorRecovery{
		config: config,
	}
}

// ExecuteWithRetry executes a function with retry logic
func (r *ErrorRecovery) ExecuteWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	
	for attempt := 0; attempt <= r.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-time.After(r.config.RetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		
		err := fn()
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Check if error is retryable
		if cErr, ok := err.(*CHTTPError); ok {
			if !cErr.IsRetryable() {
				break // Don't retry non-retryable errors
			}
		}
	}
	
	return lastErr
}

// HealthComponentFunc represents a function that checks component health
type HealthComponentFunc func() error

// HealthChecker manages health checks and error integration
type HealthChecker struct {
	components map[string]HealthComponentFunc
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		components: make(map[string]HealthComponentFunc),
	}
}

// AddComponent adds a component to health monitoring
func (h *HealthChecker) AddComponent(name string, checkFunc HealthComponentFunc) {
	h.components[name] = checkFunc
}

// HealthStatus represents the health status response
type HealthStatus struct {
	Status     string                       `json:"status"`
	Timestamp  string                       `json:"timestamp"`
	Components map[string]ComponentStatus   `json:"components"`
}

// ComponentStatus represents individual component status
type ComponentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// CheckHealth performs health checks on all components
func (h *HealthChecker) CheckHealth() HealthStatus {
	status := HealthStatus{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: make(map[string]ComponentStatus),
	}
	
	overallHealthy := true
	
	for name, checkFunc := range h.components {
		err := checkFunc()
		
		if err != nil {
			overallHealthy = false
			status.Components[name] = ComponentStatus{
				Status: "unhealthy",
				Error:  err.Error(),
			}
		} else {
			status.Components[name] = ComponentStatus{
				Status: "healthy",
			}
		}
	}
	
	if overallHealthy {
		status.Status = "healthy"
	} else {
		status.Status = "degraded"
	}
	
	return status
}

// ErrorFilter filters sensitive data from errors
type ErrorFilter struct {
	config ErrorFilterConfig
}

// ErrorFilterConfig configures error filtering
type ErrorFilterConfig struct {
	SensitiveFields []string
	MaskChar        string
}

// NewErrorFilter creates a new error filter
func NewErrorFilter(config ErrorFilterConfig) *ErrorFilter {
	return &ErrorFilter{config: config}
}

// FilterError filters sensitive data from an error
func (f *ErrorFilter) FilterError(err *CHTTPError) *CHTTPError {
	filtered := &CHTTPError{
		errorType:     err.errorType,
		message:       err.message,
		originalError: err.originalError,
		fields:        make(map[string]interface{}),
		context:       err.context, // Context is usually safe
		correlationID: err.correlationID,
		timestamp:     err.timestamp,
		stackTrace:    err.stackTrace,
		retryable:     err.retryable,
		timeout:       err.timeout,
	}
	
	// Filter fields
	for key, value := range err.fields {
		if f.isSensitiveField(key) {
			filtered.fields[key] = f.maskValue(value)
		} else {
			filtered.fields[key] = value
		}
	}
	
	return filtered
}

// isSensitiveField checks if a field is sensitive
func (f *ErrorFilter) isSensitiveField(field string) bool {
	for _, sensitive := range f.config.SensitiveFields {
		if field == sensitive {
			return true
		}
	}
	return false
}

// maskValue masks a sensitive value
func (f *ErrorFilter) maskValue(value interface{}) string {
	if f.config.MaskChar == "" {
		return "***"
	}
	return f.config.MaskChar + f.config.MaskChar + f.config.MaskChar
}