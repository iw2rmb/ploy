package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PerformHealthCheck executes a comprehensive health check.
func (h *HealthChecker) PerformHealthCheck(ctx context.Context) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		Timestamp: start,
		Status:    HealthStatusHealthy,
		Checks:    make(map[string]CheckResult),
	}

	h.checkConnectivity(ctx, result)
	h.checkConfiguration(ctx, result)

	if h.config.EnableDeepCheck {
		h.checkStorageOperations(ctx, result)
	}

	result.Metrics = h.metrics.GetSnapshot()
	result.ResponseTime = time.Since(start)
	h.determineOverallStatus(result)

	return result
}

func (h *HealthChecker) determineOverallStatus(result *HealthCheckResult) {
	var messages []string

	for checkName, check := range result.Checks {
		messages = append(messages, fmt.Sprintf("%s: %s", checkName, check.Message))

		switch check.Status {
		case HealthStatusUnhealthy:
			result.Status = HealthStatusUnhealthy
		case HealthStatusDegraded:
			if result.Status == HealthStatusHealthy {
				result.Status = HealthStatusDegraded
			}
		}
	}

	metricsStatus := result.Metrics.HealthStatus
	if metricsStatus == HealthStatusUnhealthy {
		result.Status = HealthStatusUnhealthy
	} else if metricsStatus == HealthStatusDegraded && result.Status == HealthStatusHealthy {
		result.Status = HealthStatusDegraded
	}

	result.Summary = strings.Join(messages, "; ")
	switch result.Status {
	case HealthStatusHealthy:
		result.Summary = "Storage system is healthy. " + result.Summary
	case HealthStatusDegraded:
		result.Summary = "Storage system is degraded. " + result.Summary
	case HealthStatusUnhealthy:
		result.Summary = "Storage system is unhealthy. " + result.Summary
	}
}
