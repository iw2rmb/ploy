package storage

import "time"

// HealthCheckConfig configures health check behavior.
type HealthCheckConfig struct {
	CheckInterval   time.Duration `json:"check_interval"`
	Timeout         time.Duration `json:"timeout"`
	TestBucket      string        `json:"test_bucket"`
	TestObjectSize  int64         `json:"test_object_size"`
	EnableDeepCheck bool          `json:"enable_deep_check"`
}

// DefaultHealthCheckConfig returns default health check configuration.
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		CheckInterval:   5 * time.Minute,
		Timeout:         30 * time.Second,
		TestBucket:      "ploy-health-check",
		TestObjectSize:  1024,
		EnableDeepCheck: true,
	}
}

// HealthChecker provides comprehensive storage health checking.
type HealthChecker struct {
	client  StorageProvider
	metrics *StorageMetrics
	config  *HealthCheckConfig
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(client StorageProvider, metrics *StorageMetrics, config *HealthCheckConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	return &HealthChecker{
		client:  client,
		metrics: metrics,
		config:  config,
	}
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	Timestamp    time.Time              `json:"timestamp"`
	Status       HealthStatus           `json:"status"`
	ResponseTime time.Duration          `json:"response_time"`
	Checks       map[string]CheckResult `json:"checks"`
	Summary      string                 `json:"summary"`
	Metrics      *StorageMetrics        `json:"metrics,omitempty"`
}

// CheckResult represents the result of an individual check.
type CheckResult struct {
	Status   HealthStatus  `json:"status"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}
