package arf

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealingMetricsExporter_Initialization(t *testing.T) {
	exporter := NewHealingMetricsExporter()
	require.NotNil(t, exporter)

	// Verify all metrics are initialized
	assert.NotNil(t, exporter.transformationsTotal)
	assert.NotNil(t, exporter.healingAttemptsTotal)
	assert.NotNil(t, exporter.childrenTreeDepth)
	assert.NotNil(t, exporter.healingDuration)
	assert.NotNil(t, exporter.consulOperations)
	assert.NotNil(t, exporter.llmAPICalls)
	assert.NotNil(t, exporter.queueSize)
	assert.NotNil(t, exporter.activeWorkers)
	assert.NotNil(t, exporter.circuitBreakerState)
	assert.NotNil(t, exporter.llmCostDollars)
	assert.NotNil(t, exporter.healingSuccessRate)
}

func TestHealingMetricsExporter_TransformationMetrics(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Test transformation counter
	exporter.RecordTransformationStarted("test-transform-1")
	exporter.RecordTransformationStarted("test-transform-2")

	// Get metric value
	metric := &dto.Metric{}
	counter, _ := exporter.transformationsTotal.GetMetricWithLabelValues()
	counter.Write(metric)
	assert.Equal(t, float64(2), *metric.Counter.Value)
}

func TestHealingMetricsExporter_HealingAttempts(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Record various healing attempts
	exporter.RecordHealingAttempt("success", "")
	exporter.RecordHealingAttempt("success", "")
	exporter.RecordHealingAttempt("failure", "compilation")
	exporter.RecordHealingAttempt("failure", "test")
	exporter.RecordHealingAttempt("timeout", "")

	// Verify success count
	successMetric := &dto.Metric{}
	counter, _ := exporter.healingAttemptsTotal.GetMetricWithLabelValues("success", "")
	counter.Write(successMetric)
	assert.Equal(t, float64(2), *successMetric.Counter.Value)

	// Verify failure counts by error type
	compileFailMetric := &dto.Metric{}
	counter, _ = exporter.healingAttemptsTotal.GetMetricWithLabelValues("failure", "compilation")
	counter.Write(compileFailMetric)
	assert.Equal(t, float64(1), *compileFailMetric.Counter.Value)
}

func TestHealingMetricsExporter_TreeDepth(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Record various tree depths
	exporter.RecordTreeDepth(1)
	exporter.RecordTreeDepth(3)
	exporter.RecordTreeDepth(5)
	exporter.RecordTreeDepth(10)

	// Get histogram metric - we need to collect via registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter.childrenTreeDepth)

	metricFamilies, _ := registry.Gather()
	var metric *dto.Metric
	for _, mf := range metricFamilies {
		if *mf.Name == "arf_children_tree_depth" {
			metric = mf.Metric[0]
			break
		}
	}
	require.NotNil(t, metric)

	// Should have 4 observations
	assert.Equal(t, uint64(4), *metric.Histogram.SampleCount)
	// Sum should be 19 (1+3+5+10)
	assert.Equal(t, float64(19), *metric.Histogram.SampleSum)
}

func TestHealingMetricsExporter_Duration(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Record healing durations
	exporter.RecordHealingDuration(5 * time.Second)
	exporter.RecordHealingDuration(10 * time.Second)
	exporter.RecordHealingDuration(30 * time.Second)

	// Get histogram metric - we need to collect via registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter.healingDuration)

	metricFamilies, _ := registry.Gather()
	var metric *dto.Metric
	for _, mf := range metricFamilies {
		if *mf.Name == "arf_healing_duration_seconds" {
			metric = mf.Metric[0]
			break
		}
	}
	require.NotNil(t, metric)

	assert.Equal(t, uint64(3), *metric.Histogram.SampleCount)
	assert.Equal(t, float64(45), *metric.Histogram.SampleSum) // 5+10+30
}

func TestHealingMetricsExporter_QueueAndWorkers(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Update queue size
	exporter.SetQueueSize(5)
	queueMetric := &dto.Metric{}
	exporter.queueSize.Write(queueMetric)
	assert.Equal(t, float64(5), *queueMetric.Gauge.Value)

	// Update active workers
	exporter.SetActiveWorkers(3)
	workerMetric := &dto.Metric{}
	exporter.activeWorkers.Write(workerMetric)
	assert.Equal(t, float64(3), *workerMetric.Gauge.Value)
}

func TestHealingMetricsExporter_CircuitBreaker(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Test different circuit breaker states
	tests := []struct {
		state    string
		expected float64
	}{
		{"closed", 0},
		{"open", 1},
		{"half-open", 2},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			exporter.SetCircuitBreakerState(tt.state)

			metric := &dto.Metric{}
			exporter.circuitBreakerState.Write(metric)
			assert.Equal(t, tt.expected, *metric.Gauge.Value)
		})
	}
}

func TestHealingMetricsExporter_LLMMetrics(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Record LLM API calls
	exporter.RecordLLMCall("gpt-4", true, 0)
	exporter.RecordLLMCall("gpt-4", false, 0.05)
	exporter.RecordLLMCall("gpt-3.5", false, 0.02)

	// Check API call counts
	gpt4Metric := &dto.Metric{}
	counter, _ := exporter.llmAPICalls.GetMetricWithLabelValues("gpt-4", "false")
	counter.Write(gpt4Metric)
	assert.Equal(t, float64(1), *gpt4Metric.Counter.Value)

	// Check cache hits
	cacheHitMetric := &dto.Metric{}
	counter, _ = exporter.llmAPICalls.GetMetricWithLabelValues("gpt-4", "true")
	counter.Write(cacheHitMetric)
	assert.Equal(t, float64(1), *cacheHitMetric.Counter.Value)

	// Check total cost
	costMetric := &dto.Metric{}
	exporter.llmCostDollars.Write(costMetric)
	assert.InDelta(t, 0.07, *costMetric.Counter.Value, 0.001) // 0.05 + 0.02
}

func TestHealingMetricsExporter_ConsulOperations(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Record various Consul operations
	exporter.RecordConsulOperation("get", "success")
	exporter.RecordConsulOperation("get", "success")
	exporter.RecordConsulOperation("put", "success")
	exporter.RecordConsulOperation("get", "error")
	exporter.RecordConsulOperation("delete", "success")

	// Check get success count
	getSuccessMetric := &dto.Metric{}
	counter, _ := exporter.consulOperations.GetMetricWithLabelValues("get", "success")
	counter.Write(getSuccessMetric)
	assert.Equal(t, float64(2), *getSuccessMetric.Counter.Value)

	// Check get error count
	getErrorMetric := &dto.Metric{}
	counter, _ = exporter.consulOperations.GetMetricWithLabelValues("get", "error")
	counter.Write(getErrorMetric)
	assert.Equal(t, float64(1), *getErrorMetric.Counter.Value)
}

func TestHealingMetricsExporter_SuccessRate(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Set success rate
	exporter.SetSuccessRate(0.85) // 85% success rate

	metric := &dto.Metric{}
	exporter.healingSuccessRate.Write(metric)
	assert.Equal(t, float64(85), *metric.Gauge.Value) // Stored as percentage
}

func TestHealingMetricsExporter_Registration(t *testing.T) {
	// Create a test registry
	registry := prometheus.NewRegistry()

	// Create and register exporter
	exporter := NewHealingMetricsExporter()
	err := exporter.Register(registry)
	require.NoError(t, err)

	// Add some data to ensure metrics appear in gather
	exporter.RecordTransformationStarted("test")
	exporter.RecordHealingAttempt("success", "")
	exporter.RecordTreeDepth(1)
	exporter.RecordHealingDuration(1 * time.Second)
	exporter.RecordConsulOperation("get", "success")
	exporter.RecordLLMCall("gpt-4", false, 0.01)
	exporter.SetQueueSize(0)
	exporter.SetActiveWorkers(0)
	exporter.SetCircuitBreakerState("closed")
	exporter.SetSuccessRate(1.0)

	// Verify metrics are registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Check that our metrics exist
	metricNames := make(map[string]bool)
	for _, mf := range metricFamilies {
		metricNames[*mf.Name] = true
	}

	expectedMetrics := []string{
		"arf_transformations_total",
		"arf_healing_attempts_total",
		"arf_children_tree_depth",
		"arf_healing_duration_seconds",
		"arf_consul_operations_total",
		"arf_llm_api_calls_total",
		"arf_healing_queue_size",
		"arf_healing_active_workers",
		"arf_circuit_breaker_state",
		"arf_llm_cost_dollars",
		"arf_healing_success_rate",
	}

	for _, expected := range expectedMetrics {
		assert.True(t, metricNames[expected], "Metric %s should be registered", expected)
	}
}

func TestHealingMetricsExporter_UpdateFromCoordinatorMetrics(t *testing.T) {
	exporter := NewHealingMetricsExporter()

	// Create sample coordinator metrics
	coordinatorMetrics := HealingCoordinatorMetrics{
		ActiveWorkers:       5,
		QueuedTasks:         10,
		CompletedTasks:      100,
		FailedTasks:         20,
		SuccessRate:         0.833, // ~83.3%
		CircuitBreakerState: "open",
		TotalLLMCalls:       50,
		TotalLLMCost:        2.50,
	}

	// Update exporter from coordinator metrics
	exporter.UpdateFromCoordinatorMetrics(coordinatorMetrics)

	// Verify gauge updates
	queueMetric := &dto.Metric{}
	exporter.queueSize.Write(queueMetric)
	assert.Equal(t, float64(10), *queueMetric.Gauge.Value)

	workerMetric := &dto.Metric{}
	exporter.activeWorkers.Write(workerMetric)
	assert.Equal(t, float64(5), *workerMetric.Gauge.Value)

	successRateMetric := &dto.Metric{}
	exporter.healingSuccessRate.Write(successRateMetric)
	assert.InDelta(t, 83.3, *successRateMetric.Gauge.Value, 0.1)

	circuitMetric := &dto.Metric{}
	exporter.circuitBreakerState.Write(circuitMetric)
	assert.Equal(t, float64(1), *circuitMetric.Gauge.Value) // "open" = 1
}

func TestHealingMetricsExporter_ConcurrentUpdates(t *testing.T) {
	exporter := NewHealingMetricsExporter()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Simulate concurrent metric updates
	done := make(chan bool)

	// Multiple goroutines updating different metrics
	go func() {
		for i := 0; i < 100; i++ {
			exporter.RecordTransformationStarted("transform-" + string(rune(i)))
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				exporter.RecordHealingAttempt("success", "")
			} else {
				exporter.RecordHealingAttempt("failure", "test")
			}
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			exporter.SetQueueSize(i)
			exporter.SetActiveWorkers(i % 10)
			time.Sleep(2 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines or timeout
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}
	}

	// Verify metrics were updated (no panics or race conditions)
	metric := &dto.Metric{}
	counter, _ := exporter.transformationsTotal.GetMetricWithLabelValues()
	counter.Write(metric)
	assert.Greater(t, *metric.Counter.Value, float64(0))
}
