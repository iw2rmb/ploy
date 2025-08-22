package arf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MonitoringService provides comprehensive monitoring and metrics collection for ARF
type MonitoringService interface {
	RecordTransformation(ctx context.Context, transformation TransformationMetrics) error
	RecordCircuitBreakerEvent(ctx context.Context, event CircuitBreakerEvent) error
	RecordNodeMetrics(ctx context.Context, nodeID string, metrics NodeMetrics) error
	RecordWorkloadDistribution(ctx context.Context, distribution WorkloadDistributionMetrics) error
	GetMetrics(ctx context.Context, query MetricsQuery) (*MetricsResult, error)
	GetHealthSummary(ctx context.Context) (*HealthSummary, error)
	StartMetricsCollection(ctx context.Context) error
	StopMetricsCollection() error
}

// TransformationMetrics contains metrics for a single transformation
type TransformationMetrics struct {
	TransformationID string            `json:"transformation_id"`
	RecipeID         string            `json:"recipe_id"`
	Language         string            `json:"language"`
	Success          bool              `json:"success"`
	ExecutionTime    time.Duration     `json:"execution_time"`
	ChangesApplied   int               `json:"changes_applied"`
	FilesModified    int               `json:"files_modified"`
	ErrorsCount      int               `json:"errors_count"`
	WarningsCount    int               `json:"warnings_count"`
	ValidationScore  float64           `json:"validation_score"`
	NodeID           string            `json:"node_id"`
	Timestamp        time.Time         `json:"timestamp"`
	Labels           map[string]string `json:"labels"`
}

// CircuitBreakerEvent represents a circuit breaker state change or metric
type CircuitBreakerEvent struct {
	CircuitID        string            `json:"circuit_id"`
	EventType        string            `json:"event_type"` // state_change, execution, failure
	OldState         string            `json:"old_state,omitempty"`
	NewState         string            `json:"new_state,omitempty"`
	SuccessCount     int64             `json:"success_count"`
	FailureCount     int64             `json:"failure_count"`
	FailureRate      float64           `json:"failure_rate"`
	ResponseTime     time.Duration     `json:"response_time"`
	Timestamp        time.Time         `json:"timestamp"`
	Labels           map[string]string `json:"labels"`
}

// WorkloadDistributionMetrics contains metrics for workload distribution
type WorkloadDistributionMetrics struct {
	WorkloadID               string            `json:"workload_id"`
	WorkloadType             string            `json:"workload_type"`
	NodesAssigned            int               `json:"nodes_assigned"`
	DistributionTime         time.Duration     `json:"distribution_time"`
	EstimatedCompletionTime  time.Duration     `json:"estimated_completion_time"`
	ActualCompletionTime     time.Duration     `json:"actual_completion_time,omitempty"`
	LoadBalanceScore         float64           `json:"load_balance_score"`
	FailoverNodesAvailable   int               `json:"failover_nodes_available"`
	NodeFailuresHandled      int               `json:"node_failures_handled"`
	RebalancingTriggered     bool              `json:"rebalancing_triggered"`
	Timestamp                time.Time         `json:"timestamp"`
	Labels                   map[string]string `json:"labels"`
}

// MetricsQuery defines parameters for querying metrics
type MetricsQuery struct {
	MetricType  string            `json:"metric_type"`
	TimeRange   TimeRange         `json:"time_range"`
	Filters     map[string]string `json:"filters"`
	Aggregation string            `json:"aggregation"` // sum, avg, count, rate
	GroupBy     []string          `json:"group_by"`
	Limit       int               `json:"limit"`
}

// TimeRange specifies a time range for queries
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MetricsResult contains the result of a metrics query
type MetricsResult struct {
	MetricType  string              `json:"metric_type"`
	TimeRange   TimeRange           `json:"time_range"`
	DataPoints  []MetricDataPoint   `json:"data_points"`
	Summary     MetricSummary       `json:"summary"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// MetricDataPoint represents a single data point in time series
type MetricDataPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

// MetricSummary provides aggregated summary statistics
type MetricSummary struct {
	Count   int64   `json:"count"`
	Sum     float64 `json:"sum"`
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Rate    float64 `json:"rate"` // per second
}

// HealthSummary provides an overview of system health
type HealthSummary struct {
	OverallStatus       string                 `json:"overall_status"`
	ComponentStatuses   map[string]string      `json:"component_statuses"`
	ActiveAlerts        []Alert                `json:"active_alerts"`
	MetricsSummary      SystemMetricsSummary   `json:"metrics_summary"`
	LastUpdate          time.Time              `json:"last_update"`
	UptimeSeconds       int64                  `json:"uptime_seconds"`
}

// Alert represents a system alert
type Alert struct {
	ID          string            `json:"id"`
	Level       AlertLevel        `json:"level"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Component   string            `json:"component"`
	Timestamp   time.Time         `json:"timestamp"`
	Labels      map[string]string `json:"labels"`
	Resolved    bool              `json:"resolved"`
}

// AlertLevel represents the severity level of an alert
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelError    AlertLevel = "error"
	AlertLevelCritical AlertLevel = "critical"
)

// SystemMetricsSummary provides high-level system metrics
type SystemMetricsSummary struct {
	TransformationsTotal    int64   `json:"transformations_total"`
	TransformationSuccessRate float64 `json:"transformation_success_rate"`
	AverageExecutionTime    time.Duration `json:"average_execution_time"`
	ActiveNodes             int     `json:"active_nodes"`
	TotalCapacity           int     `json:"total_capacity"`
	UtilizationRate         float64 `json:"utilization_rate"`
	CircuitBreakersOpen     int     `json:"circuit_breakers_open"`
	ActiveWorkloads         int     `json:"active_workloads"`
	QueuedWorkloads         int     `json:"queued_workloads"`
}

// DefaultMonitoringService implements the MonitoringService interface
type DefaultMonitoringService struct {
	// Storage for metrics (in production, this would be a time-series database)
	transformationMetrics     []TransformationMetrics
	circuitBreakerEvents      []CircuitBreakerEvent
	nodeMetricsHistory        map[string][]NodeMetrics
	workloadDistributionMetrics []WorkloadDistributionMetrics
	
	// System state
	alerts           []Alert
	componentStatuses map[string]string
	startTime        time.Time
	isCollecting     bool
	
	mutex sync.RWMutex
}

// NewMonitoringService creates a new monitoring service
func NewMonitoringService() MonitoringService {
	return &DefaultMonitoringService{
		transformationMetrics:       make([]TransformationMetrics, 0),
		circuitBreakerEvents:        make([]CircuitBreakerEvent, 0),
		nodeMetricsHistory:          make(map[string][]NodeMetrics),
		workloadDistributionMetrics: make([]WorkloadDistributionMetrics, 0),
		alerts:                      make([]Alert, 0),
		componentStatuses: map[string]string{
			"arf_engine":           "healthy",
			"circuit_breaker":      "healthy",
			"parallel_resolver":    "healthy",
			"multi_repo_orchestrator": "healthy",
			"ha_integration":       "healthy",
			"monitoring":           "healthy",
		},
		startTime: time.Now(),
	}
}

// RecordTransformation records metrics for a transformation execution
func (ms *DefaultMonitoringService) RecordTransformation(ctx context.Context, transformation TransformationMetrics) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	// Set timestamp if not provided
	if transformation.Timestamp.IsZero() {
		transformation.Timestamp = time.Now()
	}
	
	// Validate required fields
	if transformation.TransformationID == "" {
		return fmt.Errorf("transformation ID is required")
	}
	
	if transformation.RecipeID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	
	// Store metrics
	ms.transformationMetrics = append(ms.transformationMetrics, transformation)
	
	// Trigger alerts for failures
	if !transformation.Success {
		alert := Alert{
			ID:          fmt.Sprintf("transformation_failure_%s", transformation.TransformationID),
			Level:       AlertLevelError,
			Title:       "Transformation Failed",
			Description: fmt.Sprintf("Transformation %s using recipe %s failed", transformation.TransformationID, transformation.RecipeID),
			Component:   "arf_engine",
			Timestamp:   transformation.Timestamp,
			Labels: map[string]string{
				"transformation_id": transformation.TransformationID,
				"recipe_id":         transformation.RecipeID,
				"language":          transformation.Language,
				"node_id":           transformation.NodeID,
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	// Trigger alert for low validation score
	if transformation.ValidationScore < 0.5 {
		alert := Alert{
			ID:          fmt.Sprintf("low_validation_score_%s", transformation.TransformationID),
			Level:       AlertLevelWarning,
			Title:       "Low Validation Score",
			Description: fmt.Sprintf("Transformation %s has validation score %.2f", transformation.TransformationID, transformation.ValidationScore),
			Component:   "arf_engine",
			Timestamp:   transformation.Timestamp,
			Labels: map[string]string{
				"transformation_id": transformation.TransformationID,
				"recipe_id":         transformation.RecipeID,
				"validation_score":  fmt.Sprintf("%.2f", transformation.ValidationScore),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	return nil
}

// RecordCircuitBreakerEvent records circuit breaker events and metrics
func (ms *DefaultMonitoringService) RecordCircuitBreakerEvent(ctx context.Context, event CircuitBreakerEvent) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	// Validate required fields
	if event.CircuitID == "" {
		return fmt.Errorf("circuit ID is required")
	}
	
	if event.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	
	// Store event
	ms.circuitBreakerEvents = append(ms.circuitBreakerEvents, event)
	
	// Trigger alerts for circuit breaker state changes
	if event.EventType == "state_change" && event.NewState == "open" {
		alert := Alert{
			ID:          fmt.Sprintf("circuit_breaker_open_%s", event.CircuitID),
			Level:       AlertLevelError,
			Title:       "Circuit Breaker Opened",
			Description: fmt.Sprintf("Circuit breaker %s has opened due to failures", event.CircuitID),
			Component:   "circuit_breaker",
			Timestamp:   event.Timestamp,
			Labels: map[string]string{
				"circuit_id":    event.CircuitID,
				"failure_rate":  fmt.Sprintf("%.2f", event.FailureRate),
				"failure_count": fmt.Sprintf("%d", event.FailureCount),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	// Trigger alert for high failure rate
	if event.FailureRate > 0.8 {
		alert := Alert{
			ID:          fmt.Sprintf("high_failure_rate_%s", event.CircuitID),
			Level:       AlertLevelWarning,
			Title:       "High Circuit Breaker Failure Rate",
			Description: fmt.Sprintf("Circuit breaker %s has failure rate %.2f", event.CircuitID, event.FailureRate),
			Component:   "circuit_breaker",
			Timestamp:   event.Timestamp,
			Labels: map[string]string{
				"circuit_id":   event.CircuitID,
				"failure_rate": fmt.Sprintf("%.2f", event.FailureRate),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	return nil
}

// RecordNodeMetrics records metrics for a cluster node
func (ms *DefaultMonitoringService) RecordNodeMetrics(ctx context.Context, nodeID string, metrics NodeMetrics) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	if nodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	
	// Initialize node metrics history if needed
	if _, exists := ms.nodeMetricsHistory[nodeID]; !exists {
		ms.nodeMetricsHistory[nodeID] = make([]NodeMetrics, 0)
	}
	
	// Store metrics
	ms.nodeMetricsHistory[nodeID] = append(ms.nodeMetricsHistory[nodeID], metrics)
	
	// Keep only recent metrics (last 1000 data points per node)
	if len(ms.nodeMetricsHistory[nodeID]) > 1000 {
		ms.nodeMetricsHistory[nodeID] = ms.nodeMetricsHistory[nodeID][len(ms.nodeMetricsHistory[nodeID])-1000:]
	}
	
	// Trigger alerts for node issues
	if metrics.CPUUsage > 0.9 {
		alert := Alert{
			ID:          fmt.Sprintf("high_cpu_%s", nodeID),
			Level:       AlertLevelWarning,
			Title:       "High CPU Usage",
			Description: fmt.Sprintf("Node %s has CPU usage %.1f%%", nodeID, metrics.CPUUsage*100),
			Component:   "ha_integration",
			Timestamp:   time.Now(),
			Labels: map[string]string{
				"node_id":   nodeID,
				"cpu_usage": fmt.Sprintf("%.2f", metrics.CPUUsage),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	if metrics.ErrorRate > 0.1 {
		alert := Alert{
			ID:          fmt.Sprintf("high_error_rate_%s", nodeID),
			Level:       AlertLevelError,
			Title:       "High Node Error Rate",
			Description: fmt.Sprintf("Node %s has error rate %.2f", nodeID, metrics.ErrorRate),
			Component:   "ha_integration",
			Timestamp:   time.Now(),
			Labels: map[string]string{
				"node_id":    nodeID,
				"error_rate": fmt.Sprintf("%.2f", metrics.ErrorRate),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	return nil
}

// RecordWorkloadDistribution records metrics for workload distribution
func (ms *DefaultMonitoringService) RecordWorkloadDistribution(ctx context.Context, distribution WorkloadDistributionMetrics) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	// Set timestamp if not provided
	if distribution.Timestamp.IsZero() {
		distribution.Timestamp = time.Now()
	}
	
	// Validate required fields
	if distribution.WorkloadID == "" {
		return fmt.Errorf("workload ID is required")
	}
	
	// Store metrics
	ms.workloadDistributionMetrics = append(ms.workloadDistributionMetrics, distribution)
	
	// Trigger alerts for poor load balancing
	if distribution.LoadBalanceScore < 0.5 {
		alert := Alert{
			ID:          fmt.Sprintf("poor_load_balance_%s", distribution.WorkloadID),
			Level:       AlertLevelWarning,
			Title:       "Poor Load Balance",
			Description: fmt.Sprintf("Workload distribution has low load balance score %.2f", distribution.LoadBalanceScore),
			Component:   "multi_repo_orchestrator",
			Timestamp:   distribution.Timestamp,
			Labels: map[string]string{
				"workload_id":        distribution.WorkloadID,
				"load_balance_score": fmt.Sprintf("%.2f", distribution.LoadBalanceScore),
				"nodes_assigned":     fmt.Sprintf("%d", distribution.NodesAssigned),
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	// Trigger alert if no failover nodes available
	if distribution.FailoverNodesAvailable == 0 {
		alert := Alert{
			ID:          fmt.Sprintf("no_failover_nodes_%s", distribution.WorkloadID),
			Level:       AlertLevelError,
			Title:       "No Failover Nodes Available",
			Description: fmt.Sprintf("Workload %s has no failover nodes available", distribution.WorkloadID),
			Component:   "ha_integration",
			Timestamp:   distribution.Timestamp,
			Labels: map[string]string{
				"workload_id": distribution.WorkloadID,
			},
		}
		ms.alerts = append(ms.alerts, alert)
	}
	
	return nil
}

// GetMetrics queries stored metrics based on the provided parameters
func (ms *DefaultMonitoringService) GetMetrics(ctx context.Context, query MetricsQuery) (*MetricsResult, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	
	result := &MetricsResult{
		MetricType:  query.MetricType,
		TimeRange:   query.TimeRange,
		DataPoints:  make([]MetricDataPoint, 0),
		GeneratedAt: time.Now(),
	}
	
	switch query.MetricType {
	case "transformations":
		return ms.queryTransformationMetrics(query, result)
	case "circuit_breakers":
		return ms.queryCircuitBreakerMetrics(query, result)
	case "nodes":
		return ms.queryNodeMetrics(query, result)
	case "workload_distribution":
		return ms.queryWorkloadDistributionMetrics(query, result)
	default:
		return nil, fmt.Errorf("unsupported metric type: %s", query.MetricType)
	}
}

// queryTransformationMetrics queries transformation metrics
func (ms *DefaultMonitoringService) queryTransformationMetrics(query MetricsQuery, result *MetricsResult) (*MetricsResult, error) {
	var sum float64
	var count int64
	var min, max float64
	first := true
	
	for _, tm := range ms.transformationMetrics {
		// Apply time range filter
		if tm.Timestamp.Before(query.TimeRange.Start) || tm.Timestamp.After(query.TimeRange.End) {
			continue
		}
		
		// Apply filters
		if !ms.matchesFilters(tm.Labels, query.Filters) {
			continue
		}
		
		// Determine value based on aggregation type
		var value float64
		switch query.Aggregation {
		case "success_rate":
			if tm.Success {
				value = 1.0
			} else {
				value = 0.0
			}
		case "execution_time":
			value = float64(tm.ExecutionTime.Milliseconds())
		case "changes_applied":
			value = float64(tm.ChangesApplied)
		case "validation_score":
			value = tm.ValidationScore
		default:
			value = 1.0 // count
		}
		
		// Add data point
		dataPoint := MetricDataPoint{
			Timestamp: tm.Timestamp,
			Value:     value,
			Labels:    tm.Labels,
		}
		result.DataPoints = append(result.DataPoints, dataPoint)
		
		// Update summary statistics
		sum += value
		count++
		
		if first {
			min = value
			max = value
			first = false
		} else {
			if value < min {
				min = value
			}
			if value > max {
				max = value
			}
		}
		
		// Apply limit
		if query.Limit > 0 && len(result.DataPoints) >= query.Limit {
			break
		}
	}
	
	// Calculate summary
	if count > 0 {
		result.Summary = MetricSummary{
			Count:   count,
			Sum:     sum,
			Average: sum / float64(count),
			Min:     min,
			Max:     max,
			Rate:    float64(count) / query.TimeRange.End.Sub(query.TimeRange.Start).Seconds(),
		}
	}
	
	return result, nil
}

// queryCircuitBreakerMetrics queries circuit breaker metrics
func (ms *DefaultMonitoringService) queryCircuitBreakerMetrics(query MetricsQuery, result *MetricsResult) (*MetricsResult, error) {
	var sum float64
	var count int64
	
	for _, event := range ms.circuitBreakerEvents {
		// Apply time range filter
		if event.Timestamp.Before(query.TimeRange.Start) || event.Timestamp.After(query.TimeRange.End) {
			continue
		}
		
		// Apply filters
		if !ms.matchesFilters(event.Labels, query.Filters) {
			continue
		}
		
		// Determine value based on aggregation type
		var value float64
		switch query.Aggregation {
		case "failure_rate":
			value = event.FailureRate
		case "response_time":
			value = float64(event.ResponseTime.Milliseconds())
		case "failure_count":
			value = float64(event.FailureCount)
		case "success_count":
			value = float64(event.SuccessCount)
		default:
			value = 1.0 // count
		}
		
		// Add data point
		dataPoint := MetricDataPoint{
			Timestamp: event.Timestamp,
			Value:     value,
			Labels:    event.Labels,
		}
		result.DataPoints = append(result.DataPoints, dataPoint)
		
		sum += value
		count++
		
		// Apply limit
		if query.Limit > 0 && len(result.DataPoints) >= query.Limit {
			break
		}
	}
	
	// Calculate summary
	if count > 0 {
		result.Summary = MetricSummary{
			Count:   count,
			Sum:     sum,
			Average: sum / float64(count),
			Rate:    float64(count) / query.TimeRange.End.Sub(query.TimeRange.Start).Seconds(),
		}
	}
	
	return result, nil
}

// queryNodeMetrics queries node metrics
func (ms *DefaultMonitoringService) queryNodeMetrics(query MetricsQuery, result *MetricsResult) (*MetricsResult, error) {
	// Implementation for node metrics querying
	// For brevity, this is a simplified version
	return result, nil
}

// queryWorkloadDistributionMetrics queries workload distribution metrics
func (ms *DefaultMonitoringService) queryWorkloadDistributionMetrics(query MetricsQuery, result *MetricsResult) (*MetricsResult, error) {
	// Implementation for workload distribution metrics querying
	// For brevity, this is a simplified version
	return result, nil
}

// matchesFilters checks if labels match the provided filters
func (ms *DefaultMonitoringService) matchesFilters(labels map[string]string, filters map[string]string) bool {
	for filterKey, filterValue := range filters {
		labelValue, exists := labels[filterKey]
		if !exists || !ms.matchesFilter(labelValue, filterValue) {
			return false
		}
	}
	return true
}

// matchesFilter checks if a label value matches a filter value (supports wildcards)
func (ms *DefaultMonitoringService) matchesFilter(labelValue, filterValue string) bool {
	if filterValue == "*" {
		return true
	}
	
	if strings.Contains(filterValue, "*") {
		// Simple wildcard matching
		if strings.HasPrefix(filterValue, "*") && strings.HasSuffix(filterValue, "*") {
			// *pattern*
			pattern := strings.Trim(filterValue, "*")
			return strings.Contains(labelValue, pattern)
		} else if strings.HasPrefix(filterValue, "*") {
			// *pattern
			pattern := strings.TrimPrefix(filterValue, "*")
			return strings.HasSuffix(labelValue, pattern)
		} else if strings.HasSuffix(filterValue, "*") {
			// pattern*
			pattern := strings.TrimSuffix(filterValue, "*")
			return strings.HasPrefix(labelValue, pattern)
		}
	}
	
	return labelValue == filterValue
}

// GetHealthSummary returns an overall health summary of the system
func (ms *DefaultMonitoringService) GetHealthSummary(ctx context.Context) (*HealthSummary, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	
	// Count active alerts by level
	criticalAlerts := 0
	errorAlerts := 0
	warningAlerts := 0
	activeAlerts := make([]Alert, 0)
	
	for _, alert := range ms.alerts {
		if !alert.Resolved {
			activeAlerts = append(activeAlerts, alert)
			switch alert.Level {
			case AlertLevelCritical:
				criticalAlerts++
			case AlertLevelError:
				errorAlerts++
			case AlertLevelWarning:
				warningAlerts++
			}
		}
	}
	
	// Determine overall status
	var overallStatus string
	if criticalAlerts > 0 {
		overallStatus = "critical"
	} else if errorAlerts > 0 {
		overallStatus = "degraded"
	} else if warningAlerts > 0 {
		overallStatus = "warning"
	} else {
		overallStatus = "healthy"
	}
	
	// Calculate system metrics summary
	summary := ms.calculateSystemMetricsSummary()
	
	healthSummary := &HealthSummary{
		OverallStatus:     overallStatus,
		ComponentStatuses: ms.componentStatuses,
		ActiveAlerts:      activeAlerts,
		MetricsSummary:    summary,
		LastUpdate:        time.Now(),
		UptimeSeconds:     int64(time.Since(ms.startTime).Seconds()),
	}
	
	return healthSummary, nil
}

// calculateSystemMetricsSummary calculates high-level system metrics
func (ms *DefaultMonitoringService) calculateSystemMetricsSummary() SystemMetricsSummary {
	summary := SystemMetricsSummary{}
	
	// Calculate transformation metrics
	var successCount, totalCount int64
	var totalExecutionTime time.Duration
	
	for _, tm := range ms.transformationMetrics {
		totalCount++
		if tm.Success {
			successCount++
		}
		totalExecutionTime += tm.ExecutionTime
	}
	
	summary.TransformationsTotal = totalCount
	if totalCount > 0 {
		summary.TransformationSuccessRate = float64(successCount) / float64(totalCount)
		summary.AverageExecutionTime = time.Duration(int64(totalExecutionTime) / totalCount)
	}
	
	// Count circuit breakers in open state
	circuitBreakersOpen := 0
	for _, event := range ms.circuitBreakerEvents {
		if event.EventType == "state_change" && event.NewState == "open" {
			// Check if there's a subsequent close event
			found := false
			for _, laterEvent := range ms.circuitBreakerEvents {
				if laterEvent.CircuitID == event.CircuitID &&
					laterEvent.Timestamp.After(event.Timestamp) &&
					laterEvent.EventType == "state_change" &&
					laterEvent.NewState == "closed" {
					found = true
					break
				}
			}
			if !found {
				circuitBreakersOpen++
			}
		}
	}
	summary.CircuitBreakersOpen = circuitBreakersOpen
	
	// Node and workload information would come from external components
	summary.ActiveNodes = len(ms.nodeMetricsHistory)
	summary.ActiveWorkloads = len(ms.workloadDistributionMetrics)
	
	return summary
}

// StartMetricsCollection starts background metrics collection
func (ms *DefaultMonitoringService) StartMetricsCollection(ctx context.Context) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	if ms.isCollecting {
		return fmt.Errorf("metrics collection already started")
	}
	
	ms.isCollecting = true
	
	// In a real implementation, this would start background goroutines
	// for collecting metrics from various sources
	
	return nil
}

// StopMetricsCollection stops background metrics collection
func (ms *DefaultMonitoringService) StopMetricsCollection() error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	
	if !ms.isCollecting {
		return fmt.Errorf("metrics collection not started")
	}
	
	ms.isCollecting = false
	
	// In a real implementation, this would stop background goroutines
	
	return nil
}