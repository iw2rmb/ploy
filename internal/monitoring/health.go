package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the overall health status of the service
type HealthStatus struct {
	Status string            `json:"status"`
	Checks map[string]*Check `json:"checks"`
}

// Check represents a single health check result
type Check struct {
	Healthy   bool      `json:"healthy"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Health status constants
const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
)

// Interfaces for dependencies
type StorageHealthCheck interface {
	Ping(ctx context.Context) error
}

type ConsulHealthCheck interface {
	Ping(ctx context.Context) error
}

type WorkerPoolHealthCheck interface {
	GetUtilization() float64
}

// HealthChecker performs health checks on various system components
type HealthChecker struct {
	storage StorageHealthCheck
	consul  ConsulHealthCheck
	worker  WorkerPoolHealthCheck
	checks  map[string]func(context.Context) *Check
	mu      sync.RWMutex
}

// NewHealthChecker creates a new health checker with the given dependencies
func NewHealthChecker(storage StorageHealthCheck, consul ConsulHealthCheck, worker WorkerPoolHealthCheck) *HealthChecker {
	hc := &HealthChecker{
		storage: storage,
		consul:  consul,
		worker:  worker,
		checks:  make(map[string]func(context.Context) *Check),
	}

	// Register health check functions
	hc.checks["seaweedfs"] = hc.checkStorage
	hc.checks["consul"] = hc.checkConsul
	hc.checks["worker_pool"] = hc.checkWorkerPool

	return hc
}

// GetHealth performs all health checks and returns the overall status
func (hc *HealthChecker) GetHealth(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Checks: make(map[string]*Check),
	}

	// Run all checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for name, checkFunc := range hc.checks {
		wg.Add(1)
		go func(checkName string, fn func(context.Context) *Check) {
			defer wg.Done()
			
			// Run check with context
			check := fn(ctx)
			
			// Store result
			mu.Lock()
			status.Checks[checkName] = check
			mu.Unlock()
		}(name, checkFunc)
	}

	// Wait for all checks to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All checks completed
	case <-ctx.Done():
		// Context cancelled or timed out
		// Return partial results
	}

	// Determine overall status
	status.Status = hc.calculateOverallStatus(status.Checks)

	return status
}

// calculateOverallStatus determines the overall health status based on individual checks
func (hc *HealthChecker) calculateOverallStatus(checks map[string]*Check) string {
	unhealthyCount := 0
	criticalFailure := false

	for name, check := range checks {
		if !check.Healthy {
			unhealthyCount++
			// Storage is critical
			if name == "seaweedfs" {
				criticalFailure = true
			}
		}
	}

	if criticalFailure || unhealthyCount >= 2 {
		return StatusUnhealthy
	} else if unhealthyCount > 0 {
		return StatusDegraded
	}
	
	return StatusHealthy
}

// checkStorage checks the health of the storage system
func (hc *HealthChecker) checkStorage(ctx context.Context) *Check {
	check := &Check{
		Timestamp: time.Now(),
	}

	if err := hc.storage.Ping(ctx); err != nil {
		check.Healthy = false
		check.Error = err.Error()
	} else {
		check.Healthy = true
	}

	return check
}

// checkConsul checks the health of Consul
func (hc *HealthChecker) checkConsul(ctx context.Context) *Check {
	check := &Check{
		Timestamp: time.Now(),
	}

	if err := hc.consul.Ping(ctx); err != nil {
		check.Healthy = false
		check.Error = err.Error()
	} else {
		check.Healthy = true
	}

	return check
}

// checkWorkerPool checks the health of the worker pool
func (hc *HealthChecker) checkWorkerPool(ctx context.Context) *Check {
	check := &Check{
		Timestamp: time.Now(),
	}

	utilization := hc.worker.GetUtilization()
	
	// Consider unhealthy if utilization is above 90%
	if utilization > 0.9 {
		check.Healthy = false
		check.Error = fmt.Sprintf("worker pool overloaded: %.2f%% utilization", utilization*100)
	} else {
		check.Healthy = true
	}

	return check
}

// GetReadiness returns whether the service is ready to accept traffic
func (hc *HealthChecker) GetReadiness(ctx context.Context) bool {
	status := hc.GetHealth(ctx)
	// Ready if healthy or degraded
	return status.Status == StatusHealthy || status.Status == StatusDegraded
}

// GetLiveness returns whether the service is alive
func (hc *HealthChecker) GetLiveness(ctx context.Context) bool {
	status := hc.GetHealth(ctx)
	// Alive if not completely unhealthy
	return status.Status != StatusUnhealthy
}