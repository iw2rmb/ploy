package arf

import (
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitBreakerState) String() string {
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

// CircuitBreaker implements the circuit breaker pattern for healing attempts
type CircuitBreaker struct {
	state               CircuitBreakerState
	consecutiveFailures int
	lastFailureTime     time.Time
	openUntil           time.Time
	config              *HealingConfig
	mutex               sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *HealingConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

// CanExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should transition to half-open
		if now.After(cb.openUntil) {
			cb.state = CircuitHalfOpen
			return true // Allow one attempt
		}
		return false
	case CircuitHalfOpen:
		return true // Allow attempt to test recovery
	default:
		return false
	}
}

// RecordSuccess records a successful execution
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.consecutiveFailures = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// RecordFailure records a failed execution
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		// Failed while testing recovery, go back to open
		cb.state = CircuitOpen
		cb.openUntil = time.Now().Add(cb.config.CircuitOpenDuration)
	} else if cb.consecutiveFailures >= cb.config.FailureThreshold {
		// Too many failures, open the circuit
		cb.state = CircuitOpen
		cb.openUntil = time.Now().Add(cb.config.CircuitOpenDuration)
	}
}

// GetState returns the current state and metrics
func (cb *CircuitBreaker) GetState() (string, int, time.Time) {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.state.String(), cb.consecutiveFailures, cb.openUntil
}
