package arf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMonitoringServiceCreation(t *testing.T) {
	ms := NewMonitoringService()
	
	if ms == nil {
		t.Fatal("Expected non-nil monitoring service")
	}
}

func TestTransformationMetricsRecording(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Test successful transformation
	successMetrics := TransformationMetrics{
		TransformationID: "transform-1",
		RecipeID:         "recipe-cleanup-1",
		Language:         "java",
		Success:          true,
		ExecutionTime:    2 * time.Second,
		ChangesApplied:   5,
		FilesModified:    3,
		ValidationScore:  0.95,
		NodeID:           "node-1",
		Labels: map[string]string{
			"environment": "test",
			"project":     "sample-app",
		},
	}
	
	err := ms.RecordTransformation(ctx, successMetrics)
	if err != nil {
		t.Fatalf("Expected no error recording transformation, got: %v", err)
	}
	
	// Test failed transformation (should trigger alert)
	failureMetrics := TransformationMetrics{
		TransformationID: "transform-2",
		RecipeID:         "recipe-migration-1",
		Language:         "java",
		Success:          false,
		ExecutionTime:    5 * time.Second,
		ValidationScore:  0.2, // Low score should trigger another alert
		NodeID:           "node-1",
	}
	
	err = ms.RecordTransformation(ctx, failureMetrics)
	if err != nil {
		t.Fatalf("Expected no error recording failed transformation, got: %v", err)
	}
	
	// Check that alerts were created
	health, err := ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting health summary, got: %v", err)
	}
	
	if len(health.ActiveAlerts) < 2 {
		t.Errorf("Expected at least 2 alerts for failed transformation and low validation score, got %d", len(health.ActiveAlerts))
	}
	
	// Test invalid metrics
	invalidMetrics := TransformationMetrics{
		// Missing required fields
		Success: true,
	}
	
	err = ms.RecordTransformation(ctx, invalidMetrics)
	if err == nil {
		t.Error("Expected error for invalid transformation metrics")
	}
}

func TestCircuitBreakerEventRecording(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Test circuit breaker opening event
	openEvent := CircuitBreakerEvent{
		CircuitID:    "circuit-service-a",
		EventType:    "state_change",
		OldState:     "closed",
		NewState:     "open",
		FailureRate:  0.85,
		FailureCount: 17,
		SuccessCount: 3,
		ResponseTime: 2500 * time.Millisecond,
		Labels: map[string]string{
			"service": "service-a",
			"region":  "us-west-2",
		},
	}
	
	err := ms.RecordCircuitBreakerEvent(ctx, openEvent)
	if err != nil {
		t.Fatalf("Expected no error recording circuit breaker event, got: %v", err)
	}
	
	// Test high failure rate event
	highFailureEvent := CircuitBreakerEvent{
		CircuitID:   "circuit-service-b",
		EventType:   "execution",
		FailureRate: 0.9,
		FailureCount: 45,
		SuccessCount: 5,
	}
	
	err = ms.RecordCircuitBreakerEvent(ctx, highFailureEvent)
	if err != nil {
		t.Fatalf("Expected no error recording high failure rate event, got: %v", err)
	}
	
	// Check alerts were created
	health, err := ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting health summary, got: %v", err)
	}
	
	// Should have alerts for circuit breaker opening and high failure rate
	alertCount := len(health.ActiveAlerts)
	if alertCount < 2 {
		t.Errorf("Expected at least 2 circuit breaker alerts, got %d", alertCount)
	}
	
	// Test invalid event
	invalidEvent := CircuitBreakerEvent{
		// Missing required fields
		FailureRate: 0.5,
	}
	
	err = ms.RecordCircuitBreakerEvent(ctx, invalidEvent)
	if err == nil {
		t.Error("Expected error for invalid circuit breaker event")
	}
}

func TestNodeMetricsRecording(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Test normal node metrics
	normalMetrics := NodeMetrics{
		NodeID:             "node-1",
		CPUUsage:           0.45,
		MemoryUsage:        2 * 1024 * 1024 * 1024, // 2GB
		NetworkThroughput:  100 * 1024 * 1024,      // 100MB
		TasksCompleted:     150,
		TasksFailed:        5,
		AverageTaskTime:    45 * time.Second,
		ErrorRate:          0.03,
		Uptime:             24 * time.Hour,
	}
	
	err := ms.RecordNodeMetrics(ctx, "node-1", normalMetrics)
	if err != nil {
		t.Fatalf("Expected no error recording node metrics, got: %v", err)
	}
	
	// Test high CPU usage (should trigger alert)
	highCPUMetrics := NodeMetrics{
		NodeID:   "node-2",
		CPUUsage: 0.95, // High CPU usage
		ErrorRate: 0.05,
	}
	
	err = ms.RecordNodeMetrics(ctx, "node-2", highCPUMetrics)
	if err != nil {
		t.Fatalf("Expected no error recording high CPU metrics, got: %v", err)
	}
	
	// Test high error rate (should trigger alert)
	highErrorMetrics := NodeMetrics{
		NodeID:   "node-3",
		CPUUsage: 0.3,
		ErrorRate: 0.15, // High error rate
	}
	
	err = ms.RecordNodeMetrics(ctx, "node-3", highErrorMetrics)
	if err != nil {
		t.Fatalf("Expected no error recording high error rate metrics, got: %v", err)
	}
	
	// Check alerts were created
	health, err := ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting health summary, got: %v", err)
	}
	
	// Should have alerts for high CPU and high error rate
	foundHighCPU := false
	foundHighError := false
	
	for _, alert := range health.ActiveAlerts {
		if alert.Title == "High CPU Usage" {
			foundHighCPU = true
		}
		if alert.Title == "High Node Error Rate" {
			foundHighError = true
		}
	}
	
	if !foundHighCPU {
		t.Error("Expected high CPU usage alert")
	}
	
	if !foundHighError {
		t.Error("Expected high error rate alert")
	}
	
	// Test invalid node ID
	err = ms.RecordNodeMetrics(ctx, "", normalMetrics)
	if err == nil {
		t.Error("Expected error for empty node ID")
	}
}

func TestWorkloadDistributionMetricsRecording(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Test good distribution
	goodDistribution := WorkloadDistributionMetrics{
		WorkloadID:               "workload-1",
		WorkloadType:             "transformation",
		NodesAssigned:            3,
		DistributionTime:         100 * time.Millisecond,
		EstimatedCompletionTime:  5 * time.Minute,
		LoadBalanceScore:         0.85,
		FailoverNodesAvailable:   2,
		NodeFailuresHandled:      0,
		RebalancingTriggered:     false,
		Labels: map[string]string{
			"priority": "high",
			"region":   "us-east-1",
		},
	}
	
	err := ms.RecordWorkloadDistribution(ctx, goodDistribution)
	if err != nil {
		t.Fatalf("Expected no error recording workload distribution, got: %v", err)
	}
	
	// Test poor load balance (should trigger alert)
	poorBalance := WorkloadDistributionMetrics{
		WorkloadID:             "workload-2",
		WorkloadType:           "analysis",
		NodesAssigned:          1,
		LoadBalanceScore:       0.3, // Poor load balance
		FailoverNodesAvailable: 1,
	}
	
	err = ms.RecordWorkloadDistribution(ctx, poorBalance)
	if err != nil {
		t.Fatalf("Expected no error recording poor load balance, got: %v", err)
	}
	
	// Test no failover nodes (should trigger alert)
	noFailover := WorkloadDistributionMetrics{
		WorkloadID:             "workload-3",
		WorkloadType:           "validation",
		NodesAssigned:          1,
		LoadBalanceScore:       0.8,
		FailoverNodesAvailable: 0, // No failover nodes
	}
	
	err = ms.RecordWorkloadDistribution(ctx, noFailover)
	if err != nil {
		t.Fatalf("Expected no error recording no failover distribution, got: %v", err)
	}
	
	// Check alerts were created
	health, err := ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting health summary, got: %v", err)
	}
	
	// Should have alerts for poor load balance and no failover nodes
	foundPoorBalance := false
	foundNoFailover := false
	
	for _, alert := range health.ActiveAlerts {
		if alert.Title == "Poor Load Balance" {
			foundPoorBalance = true
		}
		if alert.Title == "No Failover Nodes Available" {
			foundNoFailover = true
		}
	}
	
	if !foundPoorBalance {
		t.Error("Expected poor load balance alert")
	}
	
	if !foundNoFailover {
		t.Error("Expected no failover nodes alert")
	}
	
	// Test invalid workload ID
	invalidDistribution := WorkloadDistributionMetrics{
		// Missing WorkloadID
		WorkloadType: "transformation",
	}
	
	err = ms.RecordWorkloadDistribution(ctx, invalidDistribution)
	if err == nil {
		t.Error("Expected error for invalid workload distribution")
	}
}

func TestMetricsQuerying(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Add some test data
	startTime := time.Now().Add(-time.Hour)
	endTime := time.Now()
	
	for i := 0; i < 10; i++ {
		metrics := TransformationMetrics{
			TransformationID: fmt.Sprintf("transform-%d", i),
			RecipeID:         "recipe-test",
			Language:         "java",
			Success:          i%3 != 0, // Some failures
			ExecutionTime:    time.Duration(i*100) * time.Millisecond,
			ChangesApplied:   i * 2,
			ValidationScore:  float64(i) * 0.1,
			Timestamp:        startTime.Add(time.Duration(i*5) * time.Minute),
			Labels: map[string]string{
				"environment": "test",
				"batch":       fmt.Sprintf("batch-%d", i/5),
			},
		}
		ms.RecordTransformation(ctx, metrics)
	}
	
	// Query transformation metrics
	query := MetricsQuery{
		MetricType: "transformations",
		TimeRange: TimeRange{
			Start: startTime,
			End:   endTime,
		},
		Aggregation: "success_rate",
		Filters: map[string]string{
			"environment": "test",
		},
		Limit: 5,
	}
	
	result, err := ms.GetMetrics(ctx, query)
	if err != nil {
		t.Fatalf("Expected no error querying metrics, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	
	if result.MetricType != "transformations" {
		t.Errorf("Expected metric type 'transformations', got %s", result.MetricType)
	}
	
	if len(result.DataPoints) == 0 {
		t.Error("Expected data points in result")
	}
	
	if len(result.DataPoints) > 5 {
		t.Errorf("Expected limit to be respected, got %d data points", len(result.DataPoints))
	}
	
	if result.Summary.Count == 0 {
		t.Error("Expected summary to be calculated")
	}
	
	// Test unsupported metric type
	invalidQuery := MetricsQuery{
		MetricType: "unsupported",
		TimeRange:  TimeRange{Start: startTime, End: endTime},
	}
	
	_, err = ms.GetMetrics(ctx, invalidQuery)
	if err == nil {
		t.Error("Expected error for unsupported metric type")
	}
}

func TestHealthSummary(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Initially should be healthy
	health, err := ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting health summary, got: %v", err)
	}
	
	if health.OverallStatus != "healthy" {
		t.Errorf("Expected healthy status, got %s", health.OverallStatus)
	}
	
	if len(health.ActiveAlerts) != 0 {
		t.Errorf("Expected no active alerts initially, got %d", len(health.ActiveAlerts))
	}
	
	if health.UptimeSeconds <= 0 {
		t.Error("Expected positive uptime")
	}
	
	// Add some problematic metrics to trigger alerts
	failedTransform := TransformationMetrics{
		TransformationID: "failed-transform",
		RecipeID:         "recipe-1",
		Success:          false,
		ValidationScore:  0.1,
	}
	
	ms.RecordTransformation(ctx, failedTransform)
	
	openCircuit := CircuitBreakerEvent{
		CircuitID: "circuit-1",
		EventType: "state_change",
		OldState:  "closed",
		NewState:  "open",
		FailureRate: 0.9,
	}
	
	ms.RecordCircuitBreakerEvent(ctx, openCircuit)
	
	// Check health status changed
	health, err = ms.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting updated health summary, got: %v", err)
	}
	
	if health.OverallStatus == "healthy" {
		t.Error("Expected status to change from healthy after errors")
	}
	
	if len(health.ActiveAlerts) == 0 {
		t.Error("Expected active alerts after recording errors")
	}
}

func TestFilterMatching(t *testing.T) {
	ms := NewMonitoringService().(*DefaultMonitoringService)
	
	labels := map[string]string{
		"environment": "production",
		"region":      "us-west-2",
		"service":     "api-gateway",
	}
	
	// Test exact match
	filters := map[string]string{
		"environment": "production",
	}
	
	if !ms.matchesFilters(labels, filters) {
		t.Error("Expected exact match to pass")
	}
	
	// Test no match
	filters = map[string]string{
		"environment": "staging",
	}
	
	if ms.matchesFilters(labels, filters) {
		t.Error("Expected non-match to fail")
	}
	
	// Test multiple filters
	filters = map[string]string{
		"environment": "production",
		"region":      "us-west-2",
	}
	
	if !ms.matchesFilters(labels, filters) {
		t.Error("Expected multiple filter match to pass")
	}
	
	// Test wildcard
	if !ms.matchesFilter("api-gateway", "*gateway") {
		t.Error("Expected wildcard suffix match to pass")
	}
	
	if !ms.matchesFilter("api-gateway", "api-*") {
		t.Error("Expected wildcard prefix match to pass")
	}
	
	if !ms.matchesFilter("api-gateway", "*-gate*") {
		t.Error("Expected wildcard middle match to pass")
	}
	
	if ms.matchesFilter("api-gateway", "*database*") {
		t.Error("Expected wildcard non-match to fail")
	}
}

func TestMetricsCollectionStartStop(t *testing.T) {
	ms := NewMonitoringService()
	ctx := context.Background()
	
	// Test starting collection
	err := ms.StartMetricsCollection(ctx)
	if err != nil {
		t.Fatalf("Expected no error starting collection, got: %v", err)
	}
	
	// Test starting again (should fail)
	err = ms.StartMetricsCollection(ctx)
	if err == nil {
		t.Error("Expected error starting collection twice")
	}
	
	// Test stopping collection
	err = ms.StopMetricsCollection()
	if err != nil {
		t.Fatalf("Expected no error stopping collection, got: %v", err)
	}
	
	// Test stopping again (should fail)
	err = ms.StopMetricsCollection()
	if err == nil {
		t.Error("Expected error stopping collection twice")
	}
}

func TestSystemMetricsSummaryCalculation(t *testing.T) {
	ms := NewMonitoringService().(*DefaultMonitoringService)
	ctx := context.Background()
	
	// Add some test transformations
	for i := 0; i < 10; i++ {
		metrics := TransformationMetrics{
			TransformationID: fmt.Sprintf("transform-%d", i),
			RecipeID:         "recipe-test",
			Success:          i%2 == 0, // 50% success rate
			ExecutionTime:    100 * time.Millisecond,
		}
		ms.RecordTransformation(ctx, metrics)
	}
	
	// Add circuit breaker events
	openEvent := CircuitBreakerEvent{
		CircuitID: "circuit-1",
		EventType: "state_change",
		NewState:  "open",
	}
	ms.RecordCircuitBreakerEvent(ctx, openEvent)
	
	// Add node metrics
	nodeMetrics := NodeMetrics{NodeID: "node-1", CPUUsage: 0.5}
	ms.RecordNodeMetrics(ctx, "node-1", nodeMetrics)
	ms.RecordNodeMetrics(ctx, "node-2", nodeMetrics)
	
	// Calculate summary
	summary := ms.calculateSystemMetricsSummary()
	
	if summary.TransformationsTotal != 10 {
		t.Errorf("Expected 10 total transformations, got %d", summary.TransformationsTotal)
	}
	
	if summary.TransformationSuccessRate != 0.5 {
		t.Errorf("Expected 0.5 success rate, got %f", summary.TransformationSuccessRate)
	}
	
	if summary.AverageExecutionTime != 100*time.Millisecond {
		t.Errorf("Expected 100ms average execution time, got %v", summary.AverageExecutionTime)
	}
	
	if summary.CircuitBreakersOpen != 1 {
		t.Errorf("Expected 1 open circuit breaker, got %d", summary.CircuitBreakersOpen)
	}
	
	if summary.ActiveNodes != 2 {
		t.Errorf("Expected 2 active nodes, got %d", summary.ActiveNodes)
	}
}