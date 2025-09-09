package arf

import (
	"time"
)

// HealingCoordinatorMetrics provides statistics about the coordinator's operation
type HealingCoordinatorMetrics struct {
	ActiveWorkers  int       `json:"active_workers"`
	QueuedTasks    int       `json:"queued_tasks"`
	CompletedTasks int       `json:"completed_tasks"`
	FailedTasks    int       `json:"failed_tasks"`
	TotalSubmitted int       `json:"total_submitted"`
	RejectedTasks  int       `json:"rejected_tasks"`
	LastActivity   time.Time `json:"last_activity"`
	UptimeSeconds  int64     `json:"uptime_seconds"`

	// Performance metrics
	SuccessRate            float64       `json:"success_rate"`
	AverageHealingDuration time.Duration `json:"average_healing_duration"`
	TotalHealingTime       time.Duration `json:"total_healing_time"`

	// Circuit breaker metrics
	CircuitBreakerState string    `json:"circuit_breaker_state"` // "closed", "open", "half-open"
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastFailureTime     time.Time `json:"last_failure_time,omitempty"`
	CircuitOpenUntil    time.Time `json:"circuit_open_until,omitempty"`

	// Limit enforcement metrics
	DepthLimitReached    int `json:"depth_limit_reached"`
	AttemptsLimitReached int `json:"attempts_limit_reached"`
	TimeoutExceeded      int `json:"timeout_exceeded"`

	// LLM cost tracking metrics
	TotalLLMCalls         int     `json:"total_llm_calls"`
	TotalLLMTokens        int     `json:"total_llm_tokens"`
	TotalLLMCost          float64 `json:"total_llm_cost"`
	LLMCacheHitRate       float64 `json:"llm_cache_hit_rate"`
	AverageLLMCostPerHeal float64 `json:"average_llm_cost_per_heal"`
}

// Metrics update methods for HealingCoordinator
func (c *HealingCoordinator) incrementActiveWorkers() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.ActiveWorkers++
	if c.metricsExporter != nil {
		c.metricsExporter.SetActiveWorkers(c.metrics.ActiveWorkers)
	}
}

func (c *HealingCoordinator) decrementActiveWorkers() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.ActiveWorkers--
	if c.metricsExporter != nil {
		c.metricsExporter.SetActiveWorkers(c.metrics.ActiveWorkers)
	}
}

func (c *HealingCoordinator) incrementQueuedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.QueuedTasks++
	if c.metricsExporter != nil {
		c.metricsExporter.SetQueueSize(c.metrics.QueuedTasks)
	}
}

func (c *HealingCoordinator) decrementQueuedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.QueuedTasks--
	if c.metricsExporter != nil {
		c.metricsExporter.SetQueueSize(c.metrics.QueuedTasks)
	}
}

func (c *HealingCoordinator) incrementCompletedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.CompletedTasks++
}

func (c *HealingCoordinator) incrementFailedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.FailedTasks++
}

func (c *HealingCoordinator) incrementSubmittedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.TotalSubmitted++
}

func (c *HealingCoordinator) incrementRejectedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.RejectedTasks++
}

func (c *HealingCoordinator) updateLastActivity() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.LastActivity = time.Now()
}

func (c *HealingCoordinator) incrementDepthLimitReached() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.DepthLimitReached++
}

func (c *HealingCoordinator) incrementAttemptsLimitReached() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.AttemptsLimitReached++
}

func (c *HealingCoordinator) incrementTimeoutExceeded() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.TimeoutExceeded++
}

func (c *HealingCoordinator) incrementCircuitBreakerRejections() {
	// Circuit breaker rejections are counted as rejected tasks
	// This is just for clarity in the code
}

func (c *HealingCoordinator) recordTaskDuration(duration time.Duration) {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()

	c.taskDurations = append(c.taskDurations, duration)
	c.metrics.TotalHealingTime += duration

	// Keep only last 100 durations for average calculation
	if len(c.taskDurations) > 100 {
		c.taskDurations = c.taskDurations[len(c.taskDurations)-100:]
	}

	// Calculate average
	if len(c.taskDurations) > 0 {
		var total time.Duration
		for _, d := range c.taskDurations {
			total += d
		}
		c.metrics.AverageHealingDuration = total / time.Duration(len(c.taskDurations))
	}

	// Calculate success rate
	if c.metrics.TotalSubmitted > 0 {
		c.metrics.SuccessRate = float64(c.metrics.CompletedTasks) / float64(c.metrics.CompletedTasks+c.metrics.FailedTasks)
	}
}

func (c *HealingCoordinator) updateCircuitBreakerMetrics() {
	state, failures, openUntil := c.circuitBreaker.GetState()

	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()

	c.metrics.CircuitBreakerState = state
	c.metrics.ConsecutiveFailures = failures
	if state == "open" {
		c.metrics.CircuitOpenUntil = openUntil
	}
	if failures > 0 {
		c.metrics.LastFailureTime = time.Now()
	}

	// Update Prometheus metrics
	if c.metricsExporter != nil {
		c.metricsExporter.SetCircuitBreakerState(state)
	}
}
