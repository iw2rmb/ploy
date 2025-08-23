package arf

import "time"

// OptimizerConfig represents configuration for the production optimizer
type OptimizerConfig struct {
	EnableCaching           bool          `json:"enable_caching"`
	EnableBatching          bool          `json:"enable_batching"`
	EnableCircuitBreaker    bool          `json:"enable_circuit_breaker"`
	EnableRateLimiting      bool          `json:"enable_rate_limiting"`
	EnableAutoScaling       bool          `json:"enable_auto_scaling"`
	MonitoringInterval      time.Duration `json:"monitoring_interval"`
	OptimizationInterval    time.Duration `json:"optimization_interval"`
	ResourceOptimization    bool          `json:"resource_optimization"`
	PerformanceTuning       bool          `json:"performance_tuning"`
}

// HealthStatus represents system health status
type HealthStatus struct {
	Status      string                 `json:"status"` // healthy, degraded, unhealthy
	Score       float64                `json:"score"`  // 0.0 - 1.0
	Issues      []HealthIssue          `json:"issues"`
	LastChecked time.Time              `json:"last_checked"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// HealthIssue represents a specific health issue
type HealthIssue struct {
	Component   string    `json:"component"`
	Issue       string    `json:"issue"`
	Severity    string    `json:"severity"` // info, warning, error, critical
	Timestamp   time.Time `json:"timestamp"`
	Remediation string    `json:"remediation,omitempty"`
}

// ResourceRequest represents a resource allocation request
type ResourceRequest struct {
	RequestID    string                 `json:"request_id"`
	CPU          float64                `json:"cpu"`
	Memory       int64                  `json:"memory"`
	Disk         int64                  `json:"disk"`
	Network      float64                `json:"network"`
	Duration     time.Duration          `json:"duration"`
	Priority     int                    `json:"priority"`
	Constraints  []ResourceConstraint   `json:"constraints"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ResourceAllocation represents allocated resources
type ResourceAllocation struct {
	AllocationID string                 `json:"allocation_id"`
	Request      ResourceRequest        `json:"request"`
	AllocatedAt  time.Time              `json:"allocated_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	Status       string                 `json:"status"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ResourceUsageExtended extends ResourceUsage with additional fields
type ResourceUsageExtended struct {
	ResourceUsage
	Allocations   []ResourceAllocation   `json:"allocations"`
}

// ResourceConstraint represents constraints on resource allocation
type ResourceConstraint struct {
	Type        string      `json:"type"`
	Field       string      `json:"field"`
	Operator    string      `json:"operator"`
	Value       interface{} `json:"value"`
	Required    bool        `json:"required"`
}

// ScalingPolicy defines how resources should be scaled
type ScalingPolicy struct {
	MinInstances    int                    `json:"min_instances"`
	MaxInstances    int                    `json:"max_instances"`
	TargetCPU       float64                `json:"target_cpu"`
	TargetMemory    float64                `json:"target_memory"`
	ScaleUpCooldown time.Duration          `json:"scale_up_cooldown"`
	ScaleDownCooldown time.Duration        `json:"scale_down_cooldown"`
	Metrics         []ScalingMetric        `json:"metrics"`
	Triggers        []ScalingTrigger       `json:"triggers"`
}

// ScalingMetric defines metrics used for scaling decisions
type ScalingMetric struct {
	Name       string  `json:"name"`
	Threshold  float64 `json:"threshold"`
	Operator   string  `json:"operator"`
	Weight     float64 `json:"weight"`
}

// ScalingTrigger defines triggers for scaling actions
type ScalingTrigger struct {
	Condition string        `json:"condition"`
	Action    string        `json:"action"`
	Delay     time.Duration `json:"delay"`
}

// ScheduledTask represents a task to be scheduled
type ScheduledTask struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Priority     int                    `json:"priority"`
	Resources    ResourceRequest        `json:"resources"`
	Dependencies []string               `json:"dependencies"`
	Constraints  []TaskConstraint       `json:"constraints"`
	Deadline     *time.Time             `json:"deadline,omitempty"`
	EstimatedDuration time.Duration     `json:"estimated_duration"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// TaskConstraint represents constraints on task scheduling
type TaskConstraint struct {
	Type        string      `json:"type"`
	Field       string      `json:"field"`
	Operator    string      `json:"operator"`
	Value       interface{} `json:"value"`
}

// Schedule represents an optimized schedule
type Schedule struct {
	ID          string                 `json:"id"`
	Tasks       []ScheduledTaskSlot    `json:"tasks"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Efficiency  float64                `json:"efficiency"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ScheduledTaskSlot represents a scheduled task slot
type ScheduledTaskSlot struct {
	Task      ScheduledTask `json:"task"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Instance  string        `json:"instance"`
	Status    string        `json:"status"`
}

// ServiceInstance represents a service instance
type ServiceInstance struct {
	ID            string                 `json:"id"`
	Address       string                 `json:"address"`
	Port          int                    `json:"port"`
	Health        HealthStatus           `json:"health"`
	Load          float64                `json:"load"`
	Capacity      float64                `json:"capacity"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// LoadBalancingStrategy represents a load balancing strategy
type LoadBalancingStrategy struct {
	Algorithm   string                 `json:"algorithm"`
	Instances   []ServiceInstance      `json:"instances"`
	Weights     map[string]float64     `json:"weights"`
	Rules       []LoadBalancingRule    `json:"rules"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// LoadBalancingRule represents a load balancing rule
type LoadBalancingRule struct {
	Condition string      `json:"condition"`
	Action    string      `json:"action"`
	Target    string      `json:"target"`
	Weight    float64     `json:"weight"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	HitRate       float64   `json:"hit_rate"`
	MissRate      float64   `json:"miss_rate"`
	Size          int64     `json:"size"`
	MaxSize       int64     `json:"max_size"`
	Evictions     int64     `json:"evictions"`
	LastUpdated   time.Time `json:"last_updated"`
}

// BatchItem represents an item in a batch
type BatchItem struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Data     interface{}            `json:"data"`
	Priority int                    `json:"priority"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Batch represents a collection of items to be processed together
type Batch struct {
	ID          string                 `json:"id"`
	Items       []BatchItem            `json:"items"`
	BatchType   string                 `json:"batch_type"`
	Priority    int                    `json:"priority"`
	CreatedAt   time.Time              `json:"created_at"`
	ScheduledAt time.Time              `json:"scheduled_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// BatchResult represents the result of batch processing
type BatchResult struct {
	BatchID       string                 `json:"batch_id"`
	ProcessedAt   time.Time              `json:"processed_at"`
	Duration      time.Duration          `json:"duration"`
	SuccessCount  int                    `json:"success_count"`
	FailureCount  int                    `json:"failure_count"`
	Results       []BatchItemResult      `json:"results"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// BatchItemResult represents the result of processing a single batch item
type BatchItemResult struct {
	ItemID    string      `json:"item_id"`
	Success   bool        `json:"success"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// Rate represents a rate limit
type Rate struct {
	Limit    int           `json:"limit"`
	Window   time.Duration `json:"window"`
	Burst    int           `json:"burst"`
}

// RateQuota represents current rate quota
type RateQuota struct {
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	Limit     int       `json:"limit"`
}

// RateLimitStats represents rate limiting statistics
type RateLimitStats struct {
	TotalRequests   int64     `json:"total_requests"`
	AllowedRequests int64     `json:"allowed_requests"`
	BlockedRequests int64     `json:"blocked_requests"`
	LastReset       time.Time `json:"last_reset"`
}

// Metric represents a single metric measurement
type Metric struct {
	Name      string                 `json:"name"`
	Value     float64                `json:"value"`
	Timestamp time.Time              `json:"timestamp"`
	Labels    map[string]string      `json:"labels"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// MetricQuery represents a query for metrics
type MetricQuery struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	TimeRange TimeRange         `json:"time_range"`
	Limit     int               `json:"limit"`
}

// AggregationQuery represents a query for aggregated metrics
type AggregationQuery struct {
	Query       MetricQuery    `json:"query"`
	Aggregation AggregationType `json:"aggregation"`
	GroupBy     []string       `json:"group_by"`
	Interval    time.Duration  `json:"interval"`
}

// AggregationType represents types of metric aggregation
type AggregationType string

const (
	AggregationSum     AggregationType = "sum"
	AggregationAvg     AggregationType = "avg"
	AggregationMin     AggregationType = "min"
	AggregationMax     AggregationType = "max"
	AggregationCount   AggregationType = "count"
	AggregationRate    AggregationType = "rate"
)

// AggregatedMetrics represents aggregated metric results
type AggregatedMetrics struct {
	Query       AggregationQuery       `json:"query"`
	Results     []AggregatedResult     `json:"results"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// AggregatedResult represents a single aggregated result
type AggregatedResult struct {
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
}

// DashboardConfig represents dashboard configuration
type DashboardConfig struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Panels      []PanelConfig   `json:"panels"`
	RefreshRate time.Duration   `json:"refresh_rate"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PanelConfig represents a dashboard panel configuration
type PanelConfig struct {
	Title       string        `json:"title"`
	Type        string        `json:"type"`
	Query       MetricQuery   `json:"query"`
	Aggregation AggregationType `json:"aggregation,omitempty"`
	Visualization string      `json:"visualization"`
}

// PerformanceAlert represents a performance-based alert
type PerformanceAlert struct {
	ID        string     `json:"id"`
	Metric    string     `json:"metric"`
	Threshold float64    `json:"threshold"`
	Operator  string     `json:"operator"`
	Level     AlertLevel `json:"level"`
	Duration  time.Duration `json:"duration"`
	Callback  func(Alert) `json:"-"`
}

// ExecutionOptions represents options for executing recipes
type ExecutionOptions struct {
	Priority        string                 `json:"priority"`
	MaxDuration     time.Duration          `json:"max_duration"`
	ResourceLimits  ResourceRequest        `json:"resource_limits"`
	Constraints     []ExecutionConstraint  `json:"constraints"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// ExecutionConstraint represents constraints on recipe execution
type ExecutionConstraint struct {
	Type     string      `json:"type"`
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// OptimizedExecutionPlan represents an optimized execution plan
type OptimizedExecutionPlan struct {
	RecipeID          string                 `json:"recipe_id"`
	Strategy          ExecutionStrategy      `json:"strategy"`
	ResourcePlan      ResourcePlan           `json:"resource_plan"`
	BatchStrategy     *BatchStrategy         `json:"batch_strategy,omitempty"`
	CacheStrategy     *CacheStrategy         `json:"cache_strategy,omitempty"`
	CircuitBreaker    bool                   `json:"circuit_breaker"`
	RateLimit         *Rate                  `json:"rate_limit,omitempty"`
	EstimatedDuration time.Duration          `json:"estimated_duration"`
	OptimizationScore float64                `json:"optimization_score"`
	CreatedAt         time.Time              `json:"created_at"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// ExecutionStrategy represents a strategy for executing recipes
type ExecutionStrategy struct {
	Type        string                 `json:"type"` // standard, aggressive, conservative, queued
	Parallelism int                    `json:"parallelism"`
	Priority    string                 `json:"priority"`
	Scheduling  string                 `json:"scheduling"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ResourcePlan represents a plan for resource allocation
type ResourcePlan struct {
	CPU               float64       `json:"cpu"`
	Memory            int64         `json:"memory"`
	Disk              int64         `json:"disk"`
	Network           float64       `json:"network"`
	EstimatedDuration time.Duration `json:"estimated_duration"`
	AllocationID      string        `json:"allocation_id,omitempty"`
}

// BatchStrategy represents a strategy for batching operations
type BatchStrategy struct {
	BatchSize    int           `json:"batch_size"`
	MaxWaitTime  time.Duration `json:"max_wait_time"`
	Priority     string        `json:"priority"`
	Grouping     string        `json:"grouping"`
}

// CacheStrategy represents a strategy for caching
type CacheStrategy struct {
	EnableResultCaching bool          `json:"enable_result_caching"`
	CacheTTL           time.Duration `json:"cache_ttl"`
	CacheKey           string        `json:"cache_key"`
	InvalidationRules  []string      `json:"invalidation_rules"`
}

// ExecutionMetrics represents metrics collected during execution
type ExecutionMetrics struct {
	ExecutionID       string        `json:"execution_id"`
	Timestamp         time.Time     `json:"timestamp"`
	Progress          float64       `json:"progress"`
	Duration          time.Duration `json:"duration"`
	ResourceUsage     ResourceUtilization `json:"resource_usage"`
	ThroughputRate    float64       `json:"throughput_rate"`
	ErrorRate         float64       `json:"error_rate"`
	QueueSize         int           `json:"queue_size"`
	Health            HealthStatus  `json:"health"`
}

// PerformanceReport represents a comprehensive performance report
type PerformanceReport struct {
	TimeRange         TimeRange             `json:"time_range"`
	PerformanceScore  float64               `json:"performance_score"`
	OptimizationScore float64               `json:"optimization_score"`
	Metrics           []Metric              `json:"metrics"`
	AggregatedMetrics AggregatedMetrics     `json:"aggregated_metrics"`
	Bottlenecks       []PerformanceBottleneck `json:"bottlenecks"`
	Recommendations   []PerformanceRecommendation `json:"recommendations"`
	GeneratedAt       time.Time             `json:"generated_at"`
}

// PerformanceBottleneck represents a performance bottleneck
type PerformanceBottleneck struct {
	Component   string    `json:"component"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Impact      float64   `json:"impact"`
	DetectedAt  time.Time `json:"detected_at"`
	Remediation string    `json:"remediation"`
}

// PerformanceRecommendation represents a performance recommendation
type PerformanceRecommendation struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"`
	Impact      float64   `json:"estimated_impact"`
	Effort      string    `json:"effort_level"`
	Actions     []string  `json:"recommended_actions"`
	CreatedAt   time.Time `json:"created_at"`
}