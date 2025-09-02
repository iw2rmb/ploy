package arf

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HealingMetricsExporter exports healing workflow metrics to Prometheus
type HealingMetricsExporter struct {
	// Counters
	transformationsTotal *prometheus.CounterVec
	healingAttemptsTotal *prometheus.CounterVec
	consulOperations     *prometheus.CounterVec
	llmAPICalls          *prometheus.CounterVec
	llmCostDollars       prometheus.Counter

	// Histograms
	childrenTreeDepth *prometheus.HistogramVec
	healingDuration   *prometheus.HistogramVec

	// Gauges
	queueSize           prometheus.Gauge
	activeWorkers       prometheus.Gauge
	circuitBreakerState prometheus.Gauge
	healingSuccessRate  prometheus.Gauge
}

// NewHealingMetricsExporter creates a new metrics exporter for healing workflows
func NewHealingMetricsExporter() *HealingMetricsExporter {
	return &HealingMetricsExporter{
		// Total transformations counter
		transformationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arf_transformations_total",
				Help: "Total number of transformations started",
			},
			[]string{}, // No labels for total count
		),

		// Healing attempts by result and error type
		healingAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arf_healing_attempts_total",
				Help: "Total healing attempts by result (success/failure/timeout) and error type",
			},
			[]string{"result", "error_type"},
		),

		// Children tree depth histogram
		childrenTreeDepth: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "arf_children_tree_depth",
				Help:    "Histogram of healing children tree depths",
				Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15, 20},
			},
			[]string{},
		),

		// Healing duration histogram
		healingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "arf_healing_duration_seconds",
				Help:    "Duration of healing workflows in seconds",
				Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~32k seconds
			},
			[]string{},
		),

		// Consul operations counter
		consulOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arf_consul_operations_total",
				Help: "Total Consul KV operations by type and status",
			},
			[]string{"operation", "status"},
		),

		// LLM API calls counter
		llmAPICalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arf_llm_api_calls_total",
				Help: "Total LLM API calls by model and cache hit status",
			},
			[]string{"model", "cache_hit"},
		),

		// LLM cost accumulator
		llmCostDollars: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "arf_llm_cost_dollars",
				Help: "Total accumulated LLM API costs in dollars",
			},
		),

		// Queue size gauge
		queueSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "arf_healing_queue_size",
				Help: "Current number of queued healing tasks",
			},
		),

		// Active workers gauge
		activeWorkers: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "arf_healing_active_workers",
				Help: "Current number of active healing workers",
			},
		),

		// Circuit breaker state gauge
		circuitBreakerState: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "arf_circuit_breaker_state",
				Help: "Circuit breaker state: 0=closed, 1=open, 2=half-open",
			},
		),

		// Success rate gauge
		healingSuccessRate: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "arf_healing_success_rate",
				Help: "Healing success rate as a percentage (0-100)",
			},
		),
	}
}

// Register registers all metrics with the provided registry
func (m *HealingMetricsExporter) Register(registry *prometheus.Registry) error {
	// Register counters
	if err := registry.Register(m.transformationsTotal); err != nil {
		return fmt.Errorf("failed to register transformations_total: %w", err)
	}
	if err := registry.Register(m.healingAttemptsTotal); err != nil {
		return fmt.Errorf("failed to register healing_attempts_total: %w", err)
	}
	if err := registry.Register(m.consulOperations); err != nil {
		return fmt.Errorf("failed to register consul_operations: %w", err)
	}
	if err := registry.Register(m.llmAPICalls); err != nil {
		return fmt.Errorf("failed to register llm_api_calls: %w", err)
	}
	if err := registry.Register(m.llmCostDollars); err != nil {
		return fmt.Errorf("failed to register llm_cost_dollars: %w", err)
	}

	// Register histograms
	if err := registry.Register(m.childrenTreeDepth); err != nil {
		return fmt.Errorf("failed to register children_tree_depth: %w", err)
	}
	if err := registry.Register(m.healingDuration); err != nil {
		return fmt.Errorf("failed to register healing_duration: %w", err)
	}

	// Register gauges
	if err := registry.Register(m.queueSize); err != nil {
		return fmt.Errorf("failed to register queue_size: %w", err)
	}
	if err := registry.Register(m.activeWorkers); err != nil {
		return fmt.Errorf("failed to register active_workers: %w", err)
	}
	if err := registry.Register(m.circuitBreakerState); err != nil {
		return fmt.Errorf("failed to register circuit_breaker_state: %w", err)
	}
	if err := registry.Register(m.healingSuccessRate); err != nil {
		return fmt.Errorf("failed to register healing_success_rate: %w", err)
	}

	return nil
}

// RecordTransformationStarted increments the total transformations counter
func (m *HealingMetricsExporter) RecordTransformationStarted(transformID string) {
	m.transformationsTotal.WithLabelValues().Inc()
}

// RecordHealingAttempt records a healing attempt with its result and error type
func (m *HealingMetricsExporter) RecordHealingAttempt(result, errorType string) {
	m.healingAttemptsTotal.WithLabelValues(result, errorType).Inc()
}

// RecordTreeDepth records the depth of a healing tree
func (m *HealingMetricsExporter) RecordTreeDepth(depth int) {
	m.childrenTreeDepth.WithLabelValues().Observe(float64(depth))
}

// RecordHealingDuration records the duration of a healing workflow
func (m *HealingMetricsExporter) RecordHealingDuration(duration time.Duration) {
	m.healingDuration.WithLabelValues().Observe(duration.Seconds())
}

// RecordConsulOperation records a Consul KV operation
func (m *HealingMetricsExporter) RecordConsulOperation(operation, status string) {
	m.consulOperations.WithLabelValues(operation, status).Inc()
}

// RecordLLMCall records an LLM API call
func (m *HealingMetricsExporter) RecordLLMCall(model string, cacheHit bool, cost float64) {
	cacheHitStr := "false"
	if cacheHit {
		cacheHitStr = "true"
	}
	m.llmAPICalls.WithLabelValues(model, cacheHitStr).Inc()

	// Only add cost for non-cached calls
	if !cacheHit && cost > 0 {
		m.llmCostDollars.Add(cost)
	}
}

// SetQueueSize updates the current queue size
func (m *HealingMetricsExporter) SetQueueSize(size int) {
	m.queueSize.Set(float64(size))
}

// SetActiveWorkers updates the current number of active workers
func (m *HealingMetricsExporter) SetActiveWorkers(count int) {
	m.activeWorkers.Set(float64(count))
}

// SetCircuitBreakerState updates the circuit breaker state
func (m *HealingMetricsExporter) SetCircuitBreakerState(state string) {
	var value float64
	switch state {
	case "closed":
		value = 0
	case "open":
		value = 1
	case "half-open":
		value = 2
	default:
		value = -1 // Unknown state
	}
	m.circuitBreakerState.Set(value)
}

// SetSuccessRate updates the healing success rate (as percentage 0-100)
func (m *HealingMetricsExporter) SetSuccessRate(rate float64) {
	// Convert from decimal (0.85) to percentage (85)
	m.healingSuccessRate.Set(rate * 100)
}

// UpdateFromCoordinatorMetrics updates metrics from coordinator metrics struct
func (m *HealingMetricsExporter) UpdateFromCoordinatorMetrics(metrics HealingCoordinatorMetrics) {
	// Update gauges
	m.SetQueueSize(metrics.QueuedTasks)
	m.SetActiveWorkers(metrics.ActiveWorkers)
	m.SetCircuitBreakerState(metrics.CircuitBreakerState)
	m.SetSuccessRate(metrics.SuccessRate)

	// Note: Counters and histograms should be updated incrementally during operations,
	// not from periodic metrics snapshots
}

// RecordHealingCompleted records a completed healing with its result
func (m *HealingMetricsExporter) RecordHealingCompleted(success bool, duration time.Duration, depth int) {
	result := "failure"
	if success {
		result = "success"
	}

	m.RecordHealingAttempt(result, "")
	m.RecordHealingDuration(duration)
	m.RecordTreeDepth(depth)
}

// RecordHealingFailed records a failed healing with error type
func (m *HealingMetricsExporter) RecordHealingFailed(errorType string, duration time.Duration, depth int) {
	m.RecordHealingAttempt("failure", errorType)
	m.RecordHealingDuration(duration)
	m.RecordTreeDepth(depth)
}

// RecordHealingTimeout records a timed out healing
func (m *HealingMetricsExporter) RecordHealingTimeout(duration time.Duration, depth int) {
	m.RecordHealingAttempt("timeout", "")
	m.RecordHealingDuration(duration)
	m.RecordTreeDepth(depth)
}
