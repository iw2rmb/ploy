package arf

import (
	"context"
	"time"
)

// PerformanceMonitor tracks system performance metrics
type PerformanceMonitor interface {
	CollectMetrics() (*PerformanceMetrics, error)
	StartMonitoring(ctx context.Context) error
	StopMonitoring() error
	GetHealthStatus() HealthStatus
	RegisterAlert(alert PerformanceAlert) error
}

// ResourceManager manages system resources
type ResourceManager interface {
	AllocateResources(req ResourceRequest) (*ResourceAllocation, error)
	ReleaseResources(allocationID string) error
	GetResourceUsage() (*ResourceUsageExtended, error)
	OptimizeResourceDistribution() error
	ScaleResources(ctx context.Context, scalingPolicy ScalingPolicy) error
}

// SchedulingEngine handles task scheduling and load balancing
type SchedulingEngine interface {
	ScheduleTask(task ScheduledTask) error
	GetOptimalSchedule(tasks []ScheduledTask) (*Schedule, error)
	LoadBalance(instances []ServiceInstance) (*LoadBalancingStrategy, error)
	PrioritizeTasks(tasks []ScheduledTask) ([]ScheduledTask, error)
}

// CacheManager handles intelligent caching strategies
type CacheManager interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration) error
	Invalidate(key string) error
	InvalidatePattern(pattern string) error
	GetCacheStats() CacheStats
	OptimizeCache() error
}

// BatchProcessor handles batch processing optimization
type BatchProcessor interface {
	CreateBatch(items []BatchItem) (*Batch, error)
	ProcessBatch(ctx context.Context, batch Batch) (*BatchResult, error)
	GetOptimalBatchSize(itemType string) int
	ScheduleBatch(batch Batch, schedule time.Time) error
}

// RateLimiter provides rate limiting functionality
type RateLimiter interface {
	Allow(key string) bool
	GetQuota(key string) RateQuota
	SetLimit(key string, limit Rate) error
	GetStats() RateLimitStats
}

// MetricsCollector collects and aggregates metrics
type MetricsCollector interface {
	RecordMetric(metric Metric) error
	GetMetrics(query MetricQuery) ([]Metric, error)
	GetAggregatedMetrics(query AggregationQuery) (*AggregatedMetrics, error)
	CreateDashboard(config DashboardConfig) error
}

// AlertManager handles alerts and notifications
type AlertManager interface {
	CreateAlert(alert Alert) error
	EvaluateRules(metrics PerformanceMetrics) ([]Alert, error)
	SendNotification(alert Alert) error
	GetActiveAlerts() ([]Alert, error)
}