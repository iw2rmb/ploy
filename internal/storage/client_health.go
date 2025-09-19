package storage

import (
	"context"
	"fmt"
	"time"
)

// GetProviderType returns the storage provider type.
func (e *StorageClient) GetProviderType() string {
	return e.client.GetProviderType()
}

// GetArtifactsBucket returns the artifacts bucket name.
func (e *StorageClient) GetArtifactsBucket() string {
	return e.client.GetArtifactsBucket()
}

// GetMetrics returns current storage metrics.
func (e *StorageClient) GetMetrics() *StorageMetrics {
	if e.metrics == nil {
		return nil
	}
	return e.metrics.GetSnapshot()
}

// GetHealthStatus returns current storage health status.
func (e *StorageClient) GetHealthStatus() *HealthCheckResult {
	if e.healthChecker == nil {
		return &HealthCheckResult{
			Status:    HealthStatusUnknown,
			Timestamp: time.Now(),
			Summary:   "Health checking disabled",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return e.healthChecker.PerformHealthCheck(ctx)
}

// GetMetricsJSON returns metrics as JSON string.
func (e *StorageClient) GetMetricsJSON() ([]byte, error) {
	if e.metrics == nil {
		return []byte("{}"), nil
	}
	return e.metrics.ToJSON()
}

// Health uses the health checker to determine overall health.
func (e *StorageClient) Health(ctx context.Context) error {
	healthResult := e.GetHealthStatus()
	if healthResult.Status != HealthStatusHealthy {
		return fmt.Errorf("storage unhealthy: %s", healthResult.Summary)
	}
	return nil
}

// Metrics returns metrics (for interface compatibility).
func (e *StorageClient) Metrics() *StorageMetrics {
	if e.metrics == nil {
		return NewStorageMetrics()
	}
	return e.metrics
}
