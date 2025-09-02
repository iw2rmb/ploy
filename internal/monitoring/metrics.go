package monitoring

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

var (
	// Job metrics
	JobsQueued = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "openrewrite_jobs_queued_total",
		Help: "Total number of jobs currently queued",
	}, []string{"priority"})

	JobsProcessing = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "openrewrite_jobs_processing_total",
		Help: "Total number of jobs currently being processed",
	})

	JobsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrewrite_jobs_completed_total",
		Help: "Total number of completed jobs",
	}, []string{"status", "recipe"})

	JobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "openrewrite_job_duration_seconds",
		Help:    "Time taken to process jobs",
		Buckets: prometheus.ExponentialBuckets(10, 2, 10), // 10s to ~3hrs
	}, []string{"recipe", "build_system"})

	// Transformation metrics
	TransformationSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "openrewrite_transformation_size_bytes",
		Help:    "Size of tar archives processed",
		Buckets: prometheus.ExponentialBuckets(1024*1024, 2, 10), // 1MB to 1GB
	}, []string{"build_system"})

	DiffSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "openrewrite_diff_size_bytes",
		Help:    "Size of generated diffs",
		Buckets: prometheus.ExponentialBuckets(1024, 2, 15), // 1KB to 16MB
	}, []string{"recipe"})

	// Resource metrics
	WorkerPoolUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "openrewrite_worker_pool_utilization",
		Help: "Percentage of worker pool in use",
	})

	MemoryUsage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "openrewrite_memory_usage_bytes",
		Help: "Current memory usage",
	})

	// Storage metrics
	ConsulOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrewrite_consul_operations_total",
		Help: "Total Consul operations",
	}, []string{"operation", "status"})

	SeaweedFSOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrewrite_seaweedfs_operations_total",
		Help: "Total SeaweedFS operations",
	}, []string{"operation", "status"})

	// Auto-scaling metrics
	ScalingEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrewrite_scaling_events_total",
		Help: "Total auto-scaling events",
	}, []string{"direction", "reason"})

	CurrentInstances = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "openrewrite_current_instances",
		Help: "Current number of service instances",
	})
)

// MetricsCollector manages Prometheus metrics for the OpenRewrite service
type MetricsCollector struct {
	registry *prometheus.Registry
	mu       sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	registry := prometheus.NewRegistry()
	// Register default collectors
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	// Initialize default values
	CurrentInstances.Set(1) // Default to 1 instance

	return &MetricsCollector{
		registry: registry,
	}
}

// RecordJobStart records the start of a job and returns a function to call when done
func (m *MetricsCollector) RecordJobStart(recipe, buildSystem string) func() {
	timer := prometheus.NewTimer(JobDuration.WithLabelValues(recipe, buildSystem))
	JobsProcessing.Inc()
	JobsQueued.WithLabelValues("normal").Dec()

	return func() {
		timer.ObserveDuration()
		JobsProcessing.Dec()
	}
}

// RecordJobComplete records a completed job
func (m *MetricsCollector) RecordJobComplete(status, recipe string) {
	JobsCompleted.WithLabelValues(status, recipe).Inc()
}

// RecordTransformationSize records the size of a transformation tar archive
func (m *MetricsCollector) RecordTransformationSize(size int64, buildSystem string) {
	TransformationSize.WithLabelValues(buildSystem).Observe(float64(size))
}

// RecordDiffSize records the size of a generated diff
func (m *MetricsCollector) RecordDiffSize(size int64, recipe string) {
	DiffSize.WithLabelValues(recipe).Observe(float64(size))
}

// RecordStorageOperation records a storage operation
func (m *MetricsCollector) RecordStorageOperation(storage, operation, status string) {
	switch storage {
	case "consul":
		ConsulOperations.WithLabelValues(operation, status).Inc()
	case "seaweedfs":
		SeaweedFSOperations.WithLabelValues(operation, status).Inc()
	}
}

// UpdateWorkerUtilization updates the worker pool utilization percentage
func (m *MetricsCollector) UpdateWorkerUtilization(utilization float64) {
	WorkerPoolUtilization.Set(utilization)
}

// GetWorkerUtilization returns the current worker pool utilization
func (m *MetricsCollector) GetWorkerUtilization() float64 {
	// Use a simple approach to get the current value
	// In production, this would typically be tracked separately
	metric := &dto.Metric{}
	WorkerPoolUtilization.Write(metric)
	if metric.Gauge != nil && metric.Gauge.Value != nil {
		return *metric.Gauge.Value
	}
	return 0
}

// UpdateMemoryUsage updates the current memory usage
func (m *MetricsCollector) UpdateMemoryUsage(bytes uint64) {
	MemoryUsage.Set(float64(bytes))
}

// RecordScalingEvent records an auto-scaling event
func (m *MetricsCollector) RecordScalingEvent(direction, reason string) {
	ScalingEvents.WithLabelValues(direction, reason).Inc()
}

// UpdateInstanceCount updates the current number of instances
func (m *MetricsCollector) UpdateInstanceCount(count int) {
	CurrentInstances.Set(float64(count))
}

// ResetMetrics resets all metrics to their initial state
func (m *MetricsCollector) ResetMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reset gauges
	JobsQueued.WithLabelValues("normal").Set(0)
	JobsProcessing.Set(0)
	WorkerPoolUtilization.Set(0)
	MemoryUsage.Set(0)
	CurrentInstances.Set(1) // Reset to default 1 instance
}

// GetRegistry returns the Prometheus registry
func (m *MetricsCollector) GetRegistry() *prometheus.Registry {
	return m.registry
}
