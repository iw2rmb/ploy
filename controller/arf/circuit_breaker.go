package arf

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// CircuitBreaker implements resilient failure handling to prevent cascade failures
type CircuitBreaker interface {
	Execute(ctx context.Context, operation func() error) error
	GetState() CircuitState
	GetMetrics() CircuitMetrics
	Reset() error
}

// CircuitState represents the current state of the circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// String returns the string representation of CircuitState
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitConfig defines the configuration for circuit breaker behavior
type CircuitConfig struct {
	FailureThreshold  int           `yaml:"failure_threshold"`
	OpenTimeout       time.Duration `yaml:"open_timeout"`
	MaxRetries        int           `yaml:"max_retries"`
	BackoffMultiplier float64       `yaml:"backoff_multiplier"`
	JitterEnabled     bool          `yaml:"jitter_enabled"`
	MinOpenDuration   time.Duration `yaml:"min_open_duration"`
	MaxOpenDuration   time.Duration `yaml:"max_open_duration"`
}

// CircuitMetrics provides observability into circuit breaker behavior
type CircuitMetrics struct {
	State                CircuitState  `json:"state"`
	SuccessCount         int64         `json:"success_count"`
	FailureCount         int64         `json:"failure_count"`
	ConsecutiveFailures  int64         `json:"consecutive_failures"`
	LastStateChange      time.Time     `json:"last_state_change"`
	StateChanges         int64         `json:"state_changes"`
	LastSuccess          time.Time     `json:"last_success"`
	LastFailure          time.Time     `json:"last_failure"`
	TotalRequests        int64         `json:"total_requests"`
	RejectedRequests     int64         `json:"rejected_requests"`
	FailureRate          float64       `json:"failure_rate"`
	AvgResponseTime      time.Duration `json:"avg_response_time"`
}

// DefaultCircuitBreaker implements the CircuitBreaker interface
type DefaultCircuitBreaker struct {
	config  CircuitConfig
	state   CircuitState
	metrics CircuitMetrics
	mutex   sync.RWMutex

	// Circuit state management
	lastFailureTime time.Time
	openTime        time.Time
	resetTime       time.Time
	
	// Response time tracking
	responseTimes []time.Duration
	responseIndex int
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config CircuitConfig) CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 10
	}
	if config.OpenTimeout <= 0 {
		config.OpenTimeout = 30 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.BackoffMultiplier <= 0 {
		config.BackoffMultiplier = 2.0
	}
	if config.MinOpenDuration <= 0 {
		config.MinOpenDuration = 5 * time.Second
	}
	if config.MaxOpenDuration <= 0 {
		config.MaxOpenDuration = 5 * time.Minute
	}

	return &DefaultCircuitBreaker{
		config:        config,
		state:         CircuitClosed,
		responseTimes: make([]time.Duration, 100), // Rolling window of 100 measurements
	}
}

// Execute runs the given operation with circuit breaker protection
func (cb *DefaultCircuitBreaker) Execute(ctx context.Context, operation func() error) error {
	cb.mutex.Lock()
	
	// Check if circuit is open and should remain open
	if cb.state == CircuitOpen {
		if time.Since(cb.openTime) < cb.getMinOpenDuration() {
			cb.metrics.RejectedRequests++
			cb.mutex.Unlock()
			return fmt.Errorf("circuit breaker is open (opened at %v)", cb.openTime)
		}
		// Transition to half-open for testing
		cb.transitionToHalfOpen()
	}

	// For half-open state, only allow one request at a time
	if cb.state == CircuitHalfOpen {
		// Implementation of single request in half-open state would require
		// additional synchronization for production use
	}

	cb.metrics.TotalRequests++
	cb.mutex.Unlock()

	// Execute the operation with timing
	startTime := time.Now()
	err := cb.executeWithRetries(ctx, operation)
	duration := time.Since(startTime)

	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Update response time metrics
	cb.updateResponseTime(duration)

	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// executeWithRetries implements exponential backoff with jitter
func (cb *DefaultCircuitBreaker) executeWithRetries(ctx context.Context, operation func() error) error {
	var lastErr error
	
	for attempt := 0; attempt <= cb.config.MaxRetries; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the operation
		if err := operation(); err != nil {
			lastErr = err
			
			// Don't retry on the last attempt
			if attempt == cb.config.MaxRetries {
				break
			}

			// Calculate backoff delay
			delay := cb.calculateBackoffDelay(attempt)
			
			// Create timer for delay
			timer := time.NewTimer(delay)
			defer timer.Stop()
			
			// Wait before retry
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			return nil // Success
		}
	}

	return lastErr
}

// calculateBackoffDelay computes the delay for exponential backoff with optional jitter
func (cb *DefaultCircuitBreaker) calculateBackoffDelay(attempt int) time.Duration {
	baseDelay := float64(time.Second) // 1 second base delay
	delay := baseDelay * math.Pow(cb.config.BackoffMultiplier, float64(attempt))
	
	// Cap the delay to a reasonable maximum
	maxDelay := float64(30 * time.Second)
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter if enabled
	if cb.config.JitterEnabled {
		jitter := rand.Float64() * 0.1 * delay // Up to 10% jitter
		delay = delay + jitter
	}

	return time.Duration(delay)
}

// recordSuccess updates metrics and potentially closes the circuit
func (cb *DefaultCircuitBreaker) recordSuccess() {
	cb.metrics.SuccessCount++
	cb.metrics.LastSuccess = time.Now()
	cb.metrics.ConsecutiveFailures = 0

	// If we're in half-open state and got a success, close the circuit
	if cb.state == CircuitHalfOpen {
		cb.transitionToClosed()
	}

	cb.updateFailureRate()
}

// recordFailure updates metrics and potentially opens the circuit
func (cb *DefaultCircuitBreaker) recordFailure() {
	cb.metrics.FailureCount++
	cb.metrics.LastFailure = time.Now()
	cb.metrics.ConsecutiveFailures++
	cb.lastFailureTime = time.Now()

	// Check if we should open the circuit
	if cb.metrics.ConsecutiveFailures >= int64(cb.config.FailureThreshold) {
		if cb.state == CircuitClosed {
			cb.transitionToOpen()
		} else if cb.state == CircuitHalfOpen {
			// Failed in half-open, go back to open
			cb.transitionToOpen()
		}
	}

	cb.updateFailureRate()
}

// transitionToOpen moves the circuit to open state
func (cb *DefaultCircuitBreaker) transitionToOpen() {
	cb.state = CircuitOpen
	cb.openTime = time.Now()
	cb.metrics.LastStateChange = cb.openTime
	cb.metrics.StateChanges++
}

// transitionToHalfOpen moves the circuit to half-open state
func (cb *DefaultCircuitBreaker) transitionToHalfOpen() {
	cb.state = CircuitHalfOpen
	cb.metrics.LastStateChange = time.Now()
	cb.metrics.StateChanges++
}

// transitionToClosed moves the circuit to closed state
func (cb *DefaultCircuitBreaker) transitionToClosed() {
	cb.state = CircuitClosed
	cb.metrics.LastStateChange = time.Now()
	cb.metrics.StateChanges++
	cb.resetTime = time.Now()
}

// getMinOpenDuration returns the minimum duration the circuit should stay open
func (cb *DefaultCircuitBreaker) getMinOpenDuration() time.Duration {
	// Implement adaptive open duration based on failure history
	baseTimeout := cb.config.OpenTimeout
	
	// Increase timeout based on consecutive failures (capped)
	multiplier := math.Min(float64(cb.metrics.ConsecutiveFailures)/10.0, 5.0)
	adaptiveTimeout := time.Duration(float64(baseTimeout) * (1.0 + multiplier))
	
	// Ensure it's within bounds
	if adaptiveTimeout < cb.config.MinOpenDuration {
		return cb.config.MinOpenDuration
	}
	if adaptiveTimeout > cb.config.MaxOpenDuration {
		return cb.config.MaxOpenDuration
	}
	
	return adaptiveTimeout
}

// updateResponseTime adds a new response time to the rolling window
func (cb *DefaultCircuitBreaker) updateResponseTime(duration time.Duration) {
	cb.responseTimes[cb.responseIndex] = duration
	cb.responseIndex = (cb.responseIndex + 1) % len(cb.responseTimes)
	
	// Calculate average response time
	var total time.Duration
	count := 0
	for _, rt := range cb.responseTimes {
		if rt > 0 {
			total += rt
			count++
		}
	}
	
	if count > 0 {
		cb.metrics.AvgResponseTime = total / time.Duration(count)
	}
}

// updateFailureRate calculates the current failure rate
func (cb *DefaultCircuitBreaker) updateFailureRate() {
	total := cb.metrics.SuccessCount + cb.metrics.FailureCount
	if total > 0 {
		cb.metrics.FailureRate = float64(cb.metrics.FailureCount) / float64(total)
	}
}

// GetState returns the current circuit breaker state
func (cb *DefaultCircuitBreaker) GetState() CircuitState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetMetrics returns the current circuit breaker metrics
func (cb *DefaultCircuitBreaker) GetMetrics() CircuitMetrics {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	
	// Update the state in metrics before returning
	cb.metrics.State = cb.state
	return cb.metrics
}

// Reset manually resets the circuit breaker to closed state
func (cb *DefaultCircuitBreaker) Reset() error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.transitionToClosed()
	cb.metrics.ConsecutiveFailures = 0
	
	return nil
}

// CircuitBreakerManager manages multiple circuit breakers by name
type CircuitBreakerManager struct {
	breakers map[string]CircuitBreaker
	configs  map[string]CircuitConfig
	mutex    sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]CircuitBreaker),
		configs:  make(map[string]CircuitConfig),
	}
}

// GetCircuitBreaker returns a circuit breaker by name, creating it if necessary
func (cbm *CircuitBreakerManager) GetCircuitBreaker(name string) CircuitBreaker {
	cbm.mutex.RLock()
	if breaker, exists := cbm.breakers[name]; exists {
		cbm.mutex.RUnlock()
		return breaker
	}
	cbm.mutex.RUnlock()

	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	// Double-check after acquiring write lock
	if breaker, exists := cbm.breakers[name]; exists {
		return breaker
	}

	// Create new circuit breaker with config if available
	config := cbm.configs[name]
	if config.FailureThreshold == 0 {
		// Use default config
		config = CircuitConfig{
			FailureThreshold:  10,
			OpenTimeout:       30 * time.Second,
			MaxRetries:        3,
			BackoffMultiplier: 2.0,
			JitterEnabled:     true,
			MinOpenDuration:   5 * time.Second,
			MaxOpenDuration:   5 * time.Minute,
		}
	}

	breaker := NewCircuitBreaker(config)
	cbm.breakers[name] = breaker
	return breaker
}

// SetConfig sets the configuration for a named circuit breaker
func (cbm *CircuitBreakerManager) SetConfig(name string, config CircuitConfig) {
	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()
	cbm.configs[name] = config
}

// GetAllMetrics returns metrics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllMetrics() map[string]CircuitMetrics {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	metrics := make(map[string]CircuitMetrics)
	for name, breaker := range cbm.breakers {
		metrics[name] = breaker.GetMetrics()
	}
	return metrics
}