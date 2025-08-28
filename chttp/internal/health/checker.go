package health

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/chttp/internal/config"
)

// HealthChecker provides basic health checking
type HealthChecker struct {
	config    *config.Config
	startTime time.Time
}

// HealthStatus represents health check response
type HealthStatus struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Uptime    time.Duration `json:"uptime"`
	Version   string        `json:"version"`
	Config    ConfigSummary `json:"config,omitempty"`
}

// ConfigSummary provides basic configuration information
type ConfigSummary struct {
	AllowedCommands int    `json:"allowed_commands"`
	LogLevel        string `json:"log_level"`
	Port            int    `json:"port"`
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(cfg *config.Config) *HealthChecker {
	return &HealthChecker{
		config:    cfg,
		startTime: time.Now(),
	}
}

// CheckHealth performs basic health check
func (hc *HealthChecker) CheckHealth(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(hc.startTime),
		Version:   "1.0.0",
	}

	// Add configuration summary if requested
	status.Config = ConfigSummary{
		AllowedCommands: len(hc.config.Commands.Allowed),
		LogLevel:        hc.config.Logging.Level,
		Port:            hc.config.Server.Port,
	}

	return status
}

// IsHealthy performs a basic health check
func (hc *HealthChecker) IsHealthy(ctx context.Context) bool {
	// For now, we're always healthy if we can respond
	// In the future, could check disk space, memory, etc.
	return true
}