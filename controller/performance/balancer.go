package performance

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// LoadBalancer defines the interface for request distribution
type LoadBalancer interface {
	SelectInstance(ctx context.Context) (*Instance, error)
	UpdateInstanceHealth(instanceID string, healthy bool)
	GetInstances() []*Instance
	AddInstance(instance *Instance)
	RemoveInstance(instanceID string)
}

// Instance represents a controller instance
type Instance struct {
	ID          string
	Address     string
	Port        int
	Healthy     bool
	Weight      int
	LastChecked time.Time
	
	// Performance metrics
	RequestCount int64
	ErrorCount   int64
	ResponseTime time.Duration
	
	mu sync.RWMutex
}

// UpdateHealth updates the instance health status
func (i *Instance) UpdateHealth(healthy bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Healthy = healthy
	i.LastChecked = time.Now()
}

// IncrementRequests increments the request counter
func (i *Instance) IncrementRequests() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.RequestCount++
}

// IncrementErrors increments the error counter
func (i *Instance) IncrementErrors() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ErrorCount++
}

// UpdateResponseTime updates the average response time
func (i *Instance) UpdateResponseTime(duration time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	// Simple moving average
	i.ResponseTime = (i.ResponseTime + duration) / 2
}

// GetStats returns instance statistics
func (i *Instance) GetStats() map[string]interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	return map[string]interface{}{
		"id":            i.ID,
		"address":       i.Address,
		"port":          i.Port,
		"healthy":       i.Healthy,
		"weight":        i.Weight,
		"last_checked":  i.LastChecked,
		"request_count": i.RequestCount,
		"error_count":   i.ErrorCount,
		"response_time": i.ResponseTime,
	}
}

// WeightedRoundRobin implements a weighted round-robin load balancer
type WeightedRoundRobin struct {
	instances []*Instance
	current   int
	mu        sync.RWMutex
	logger    *log.Logger
}

// NewWeightedRoundRobin creates a new weighted round-robin load balancer
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{
		instances: make([]*Instance, 0),
		logger:    log.Default(),
	}
}

// SelectInstance selects the next instance using weighted round-robin
func (wrr *WeightedRoundRobin) SelectInstance(ctx context.Context) (*Instance, error) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	if len(wrr.instances) == 0 {
		return nil, fmt.Errorf("no instances available")
	}
	
	// Filter healthy instances
	healthyInstances := make([]*Instance, 0)
	for _, instance := range wrr.instances {
		if instance.Healthy {
			healthyInstances = append(healthyInstances, instance)
		}
	}
	
	if len(healthyInstances) == 0 {
		return nil, fmt.Errorf("no healthy instances available")
	}
	
	// Simple round-robin for now (can be enhanced with weights)
	selectedInstance := healthyInstances[wrr.current%len(healthyInstances)]
	wrr.current++
	
	selectedInstance.IncrementRequests()
	return selectedInstance, nil
}

// UpdateInstanceHealth updates the health status of an instance
func (wrr *WeightedRoundRobin) UpdateInstanceHealth(instanceID string, healthy bool) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	for _, instance := range wrr.instances {
		if instance.ID == instanceID {
			instance.UpdateHealth(healthy)
			wrr.logger.Printf("Updated health for instance %s: %v", instanceID, healthy)
			return
		}
	}
	
	wrr.logger.Printf("Warning: Instance %s not found for health update", instanceID)
}

// GetInstances returns all instances
func (wrr *WeightedRoundRobin) GetInstances() []*Instance {
	wrr.mu.RLock()
	defer wrr.mu.RUnlock()
	
	// Return a copy to prevent external modification
	instances := make([]*Instance, len(wrr.instances))
	copy(instances, wrr.instances)
	return instances
}

// AddInstance adds a new instance to the load balancer
func (wrr *WeightedRoundRobin) AddInstance(instance *Instance) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	// Check if instance already exists
	for _, existing := range wrr.instances {
		if existing.ID == instance.ID {
			wrr.logger.Printf("Instance %s already exists, updating", instance.ID)
			*existing = *instance
			return
		}
	}
	
	wrr.instances = append(wrr.instances, instance)
	wrr.logger.Printf("Added instance %s to load balancer", instance.ID)
}

// RemoveInstance removes an instance from the load balancer
func (wrr *WeightedRoundRobin) RemoveInstance(instanceID string) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	
	for i, instance := range wrr.instances {
		if instance.ID == instanceID {
			// Remove instance from slice
			wrr.instances = append(wrr.instances[:i], wrr.instances[i+1:]...)
			wrr.logger.Printf("Removed instance %s from load balancer", instanceID)
			
			// Adjust current index if necessary
			if wrr.current >= len(wrr.instances) {
				wrr.current = 0
			}
			return
		}
	}
	
	wrr.logger.Printf("Warning: Instance %s not found for removal", instanceID)
}

// GetStats returns load balancer statistics
func (wrr *WeightedRoundRobin) GetStats() map[string]interface{} {
	wrr.mu.RLock()
	defer wrr.mu.RUnlock()
	
	stats := map[string]interface{}{
		"total_instances":   len(wrr.instances),
		"current_index":     wrr.current,
		"instances":         make([]map[string]interface{}, 0),
	}
	
	healthyCount := 0
	for _, instance := range wrr.instances {
		if instance.Healthy {
			healthyCount++
		}
		stats["instances"] = append(stats["instances"].([]map[string]interface{}), instance.GetStats())
	}
	
	stats["healthy_instances"] = healthyCount
	return stats
}

// CircuitBreaker implements circuit breaker pattern for external services
type CircuitBreaker struct {
	name           string
	maxFailures    int
	timeout        time.Duration
	failures       int
	lastFailTime   time.Time
	state          CircuitState
	mu             sync.RWMutex
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:        name,
		maxFailures: maxFailures,
		timeout:     timeout,
		state:       Closed,
	}
}

// Call executes a function through the circuit breaker
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker %s is open", cb.name)
	}
	
	err := fn()
	if err != nil {
		cb.recordFailure()
		return err
	}
	
	cb.recordSuccess()
	return nil
}

// canExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	if cb.state == Closed {
		return true
	}
	
	if cb.state == Open {
		if time.Since(cb.lastFailTime) > cb.timeout {
			cb.state = HalfOpen
			return true
		}
		return false
	}
	
	return true // HalfOpen state allows execution
}

// recordFailure records a failure and potentially opens the circuit
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures++
	cb.lastFailTime = time.Now()
	
	if cb.failures >= cb.maxFailures {
		cb.state = Open
	}
}

// recordSuccess records a success and potentially closes the circuit
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures = 0
	if cb.state == HalfOpen {
		cb.state = Closed
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	stateStr := "closed"
	if cb.state == Open {
		stateStr = "open"
	} else if cb.state == HalfOpen {
		stateStr = "half-open"
	}
	
	return map[string]interface{}{
		"name":           cb.name,
		"state":          stateStr,
		"failures":       cb.failures,
		"max_failures":   cb.maxFailures,
		"last_fail_time": cb.lastFailTime,
		"timeout":        cb.timeout,
	}
}