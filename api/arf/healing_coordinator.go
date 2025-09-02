package arf

import (
	"container/heap"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HealingTask represents a healing attempt to be coordinated
type HealingTask struct {
	TransformID string
	AttemptPath string
	Errors      []string
	ParentPath  string
	Priority    int // Lower values = higher priority
	SubmittedAt time.Time
	ExecuteFn   func(context.Context) error
}

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

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for healing attempts
type CircuitBreaker struct {
	state               CircuitBreakerState
	consecutiveFailures int
	lastFailureTime     time.Time
	openUntil           time.Time
	config              *HealingConfig
	mutex               sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *HealingConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

// CanExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should transition to half-open
		if now.After(cb.openUntil) {
			cb.state = CircuitHalfOpen
			return true // Allow one attempt
		}
		return false
	case CircuitHalfOpen:
		return true // Allow attempt to test recovery
	default:
		return false
	}
}

// RecordSuccess records a successful execution
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.consecutiveFailures = 0
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// RecordFailure records a failed execution
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		// Failed while testing recovery, go back to open
		cb.state = CircuitOpen
		cb.openUntil = time.Now().Add(cb.config.CircuitOpenDuration)
	} else if cb.consecutiveFailures >= cb.config.FailureThreshold {
		// Too many failures, open the circuit
		cb.state = CircuitOpen
		cb.openUntil = time.Now().Add(cb.config.CircuitOpenDuration)
	}
}

// GetState returns the current state and metrics
func (cb *CircuitBreaker) GetState() (string, int, time.Time) {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.state.String(), cb.consecutiveFailures, cb.openUntil
}

// HealingCoordinator manages parallel healing attempt execution
type HealingCoordinator struct {
	config *HealingConfig

	// Concurrency control
	semaphore   chan struct{} // Semaphore for parallel control
	taskQueue   *TaskQueue    // Priority queue for pending tasks
	activeTasks sync.Map      // Active task tracking

	// State management
	ctx     context.Context
	cancel  context.CancelFunc
	running int32
	wg      sync.WaitGroup

	// Circuit breaker
	circuitBreaker *CircuitBreaker

	// Metrics and performance tracking
	metrics         HealingCoordinatorMetrics
	metricsMutex    sync.RWMutex
	startTime       time.Time
	taskDurations   []time.Duration // Track durations for average calculation
	attemptCounters map[string]int  // Track attempts per transformation

	// LLM analyzer for cost tracking
	llmAnalyzer *EnhancedLLMAnalyzer

	// Prometheus metrics exporter
	metricsExporter *HealingMetricsExporter

	// Alert manager for monitoring and alerts
	alertManager *HealingAlertManager

	// Channels
	taskSubmissions  chan *HealingTask
	shutdownComplete chan struct{}
}

// TaskQueue implements a priority queue for healing tasks
type TaskQueue struct {
	items []*HealingTask
	mutex sync.RWMutex
}

// NewHealingCoordinator creates a new healing coordinator
func NewHealingCoordinator(config *HealingConfig) *HealingCoordinator {
	if config == nil {
		config = DefaultHealingConfig()
	}

	// Set default queue size if not specified
	queueSize := 100
	if config.QueueSize > 0 {
		queueSize = config.QueueSize
	}

	coordinator := &HealingCoordinator{
		config:           config,
		semaphore:        make(chan struct{}, config.MaxParallelAttempts),
		taskQueue:        &TaskQueue{items: make([]*HealingTask, 0, queueSize)},
		taskSubmissions:  make(chan *HealingTask, queueSize),
		shutdownComplete: make(chan struct{}),
		startTime:        time.Now(),
		circuitBreaker:   NewCircuitBreaker(config),
		taskDurations:    make([]time.Duration, 0, 100),
		attemptCounters:  make(map[string]int),
		llmAnalyzer:      NewEnhancedLLMAnalyzer(nil, nil), // Initialize with nil, can be set later
		metricsExporter:  NewHealingMetricsExporter(),      // Initialize Prometheus metrics
	}

	// Initialize alert manager if configured
	if config.AlertConfig != nil && config.AlertConfig.Enabled {
		coordinator.alertManager = NewHealingAlertManager(config.AlertConfig)
	}

	// Initialize metrics with circuit breaker state
	coordinator.metrics.CircuitBreakerState = CircuitClosed.String()

	return coordinator
}

// Start begins the coordinator's operation
func (c *HealingCoordinator) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return fmt.Errorf("coordinator already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.startTime = time.Now()

	// Log coordinator startup
	GetHealingLogger().WithFields(LogFields{
		"max_parallel_attempts": c.config.MaxParallelAttempts,
		"max_healing_depth":     c.config.MaxHealingDepth,
		"max_total_attempts":    c.config.MaxTotalAttempts,
		"attempt_timeout":       c.config.AttemptTimeout.String(),
		"failure_threshold":     c.config.FailureThreshold,
	}).Info("Healing coordinator started")

	// Start the main processing loop
	c.wg.Add(1)
	go c.processLoop()

	// Start alert manager if configured
	if c.alertManager != nil {
		if err := c.alertManager.Start(ctx); err != nil {
			GetHealingLogger().WithFields(LogFields{
				"error": err.Error(),
			}).Warn("Failed to start alert manager")
			// Don't fail coordinator startup if alert manager fails
		}
	}

	return nil
}

// Stop gracefully shuts down the coordinator
func (c *HealingCoordinator) Stop() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return // Not running
	}

	// Log coordinator shutdown
	GetHealingLogger().Info("Healing coordinator stopping")

	// Cancel context to stop accepting new tasks
	c.cancel()

	// Wait for all workers to complete
	c.wg.Wait()

	// Stop alert manager if running
	if c.alertManager != nil {
		c.alertManager.Stop()
	}

	// Log final shutdown
	GetHealingLogger().WithFields(LogFields{
		"total_tasks_processed": c.metrics.TotalSubmitted,
		"uptime_seconds":        time.Since(c.startTime).Seconds(),
	}).Info("Healing coordinator stopped")

	close(c.shutdownComplete)
}

// SubmitTask submits a healing task for execution
func (c *HealingCoordinator) SubmitTask(ctx context.Context, task *HealingTask) error {
	if atomic.LoadInt32(&c.running) == 0 {
		return fmt.Errorf("coordinator stopped")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check circuit breaker
	if !c.circuitBreaker.CanExecute() {
		c.incrementRejectedTasks()
		c.incrementCircuitBreakerRejections()
		state, failures, openUntil := c.circuitBreaker.GetState()

		// Log circuit breaker trip
		GetHealingLogger().LogCircuitBreakerTrip(
			task.TransformID,
			state,
			failures,
			time.Until(openUntil),
		)

		return fmt.Errorf("circuit breaker is %s (failures: %d, open until: %v)", state, failures, openUntil)
	}

	// Validate task and check limits
	if err := c.validateTask(task); err != nil {
		c.incrementRejectedTasks()
		return fmt.Errorf("task validation failed: %w", err)
	}

	// Check total attempts limit for this transformation
	c.metricsMutex.Lock()
	attempts := c.attemptCounters[task.TransformID]
	if attempts >= c.config.MaxTotalAttempts {
		c.metricsMutex.Unlock()
		c.incrementRejectedTasks()
		c.incrementAttemptsLimitReached()
		return fmt.Errorf("max total attempts (%d) reached for transformation %s", c.config.MaxTotalAttempts, task.TransformID)
	}
	c.attemptCounters[task.TransformID] = attempts + 1
	c.metricsMutex.Unlock()

	// Set submission time
	task.SubmittedAt = time.Now()

	// Try to submit task
	select {
	case c.taskSubmissions <- task:
		c.incrementSubmittedTasks()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return fmt.Errorf("coordinator shutting down")
	default:
		c.incrementRejectedTasks()
		return fmt.Errorf("queue full")
	}
}

// validateTask checks if a task meets the coordinator's limits
func (c *HealingCoordinator) validateTask(task *HealingTask) error {
	// Check depth limit
	depth := GetPathDepth(task.AttemptPath)
	if depth > c.config.MaxHealingDepth {
		c.incrementDepthLimitReached()
		return fmt.Errorf("max healing depth (%d) exceeded: %d", c.config.MaxHealingDepth, depth)
	}

	// Basic task validation
	if task.TransformID == "" {
		return fmt.Errorf("transformation ID is required")
	}

	if task.ExecuteFn == nil {
		return fmt.Errorf("execute function is required")
	}

	return nil
}

// processLoop is the main processing loop for the coordinator
func (c *HealingCoordinator) processLoop() {
	defer c.wg.Done()

	// Create ticker for periodic metrics logging (every 5 minutes)
	metricsTicker := time.NewTicker(5 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case task := <-c.taskSubmissions:
			c.enqueueTask(task)
			c.tryStartWorker()

		case <-metricsTicker.C:
			// Log periodic performance metrics
			metrics := c.GetMetrics()
			GetHealingLogger().LogPerformanceMetrics(metrics)

			// Update Prometheus metrics
			if c.metricsExporter != nil {
				c.metricsExporter.UpdateFromCoordinatorMetrics(metrics)
			}

			// Evaluate alert rules
			if c.alertManager != nil {
				c.alertManager.EvaluateRules(metrics)
			}

		case <-c.ctx.Done():
			// Log final metrics before shutdown
			finalMetrics := c.GetMetrics()
			GetHealingLogger().WithFields(LogFields{
				"reason": "shutdown",
			}).Info("Final healing coordinator metrics")
			GetHealingLogger().LogPerformanceMetrics(finalMetrics)

			// Process remaining queued tasks with short timeout
			c.drainQueue()
			return
		}
	}
}

// enqueueTask adds a task to the priority queue
func (c *HealingCoordinator) enqueueTask(task *HealingTask) {
	c.taskQueue.PushTask(task)
	c.incrementQueuedTasks()
}

// tryStartWorker attempts to start a worker if semaphore allows
func (c *HealingCoordinator) tryStartWorker() {
	// Try to acquire semaphore
	select {
	case c.semaphore <- struct{}{}:
		// Successfully acquired, check if we have queued tasks
		if task := c.taskQueue.PopTask(); task != nil {
			c.decrementQueuedTasks()
			c.wg.Add(1)
			go c.executeTask(task)
		} else {
			// No task available, release semaphore
			<-c.semaphore
		}
	default:
		// Semaphore full, no worker available
	}
}

// executeTask executes a single healing task
func (c *HealingCoordinator) executeTask(task *HealingTask) {
	defer func() {
		c.wg.Done()
		<-c.semaphore // Release semaphore
		c.activeTasks.Delete(task.TransformID)
		c.decrementActiveWorkers()
		c.updateLastActivity()

		// Stop tracking in alert manager
		if c.alertManager != nil {
			c.alertManager.StopTracking(task.TransformID)
		}
	}()

	// Track active task
	c.activeTasks.Store(task.TransformID, task)
	c.incrementActiveWorkers()

	// Track transformation in alert manager
	if c.alertManager != nil {
		c.alertManager.StartTracking(task.TransformID)
	}

	// Log task execution start
	GetHealingLogger().WithFields(LogFields{
		"transformation_id": task.TransformID,
		"attempt_path":      task.AttemptPath,
		"priority":          task.Priority,
		"parent_path":       task.ParentPath,
	}).Info("Starting healing task execution")

	// Create task-specific context with timeout
	taskCtx, cancel := context.WithTimeout(c.ctx, c.config.AttemptTimeout)
	defer cancel()

	// Track execution time
	startTime := time.Now()

	// Execute the task
	err := task.ExecuteFn(taskCtx)
	duration := time.Since(startTime)

	// Record task duration for metrics
	c.recordTaskDuration(duration)

	// Calculate tree depth based on attempt path
	depth := len(strings.Split(task.AttemptPath, "."))

	// Record tree depth in alert manager
	if c.alertManager != nil {
		c.alertManager.RecordTreeDepth(task.TransformID, depth)
	}

	if err != nil {
		c.incrementFailedTasks()
		c.circuitBreaker.RecordFailure()

		// Check if it was a timeout
		if taskCtx.Err() == context.DeadlineExceeded {
			c.incrementTimeoutExceeded()
			GetHealingLogger().WithFields(LogFields{
				"transformation_id": task.TransformID,
				"attempt_path":      task.AttemptPath,
				"duration_ms":       duration.Milliseconds(),
			}).Error("Healing task timed out", err)
			if c.metricsExporter != nil {
				c.metricsExporter.RecordHealingTimeout(duration, depth)
			}
		} else {
			GetHealingLogger().LogHealingFailed(c.ctx, task.TransformID, task.AttemptPath, err)
			if c.metricsExporter != nil {
				// Determine error type from error message
				errorType := "unknown"
				if strings.Contains(err.Error(), "compilation") {
					errorType = "compilation"
				} else if strings.Contains(err.Error(), "test") {
					errorType = "test"
				} else if strings.Contains(err.Error(), "build") {
					errorType = "build"
				}
				c.metricsExporter.RecordHealingFailed(errorType, duration, depth)
			}
		}
	} else {
		c.incrementCompletedTasks()
		c.circuitBreaker.RecordSuccess()
		GetHealingLogger().LogHealingCompleted(c.ctx, task.TransformID, task.AttemptPath, "success", duration)
		if c.metricsExporter != nil {
			c.metricsExporter.RecordHealingCompleted(true, duration, depth)
		}
	}

	// Update circuit breaker state in metrics
	c.updateCircuitBreakerMetrics()
}

// drainQueue processes remaining queued tasks with a short timeout
func (c *HealingCoordinator) drainQueue() {
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		task := c.taskQueue.PopTask()
		if task == nil {
			break
		}

		// Try to start worker with short timeout
		select {
		case c.semaphore <- struct{}{}:
			c.wg.Add(1)
			go func(t *HealingTask) {
				defer func() {
					c.wg.Done()
					<-c.semaphore
				}()

				// Execute with drain context
				t.ExecuteFn(drainCtx)
			}(task)

		case <-drainCtx.Done():
			// Timeout reached, abandon remaining tasks
			c.incrementRejectedTasks()
			for c.taskQueue.PopTask() != nil {
				c.incrementRejectedTasks()
			}
			return
		}
	}
}

// GetMetrics returns current coordinator metrics
func (c *HealingCoordinator) GetMetrics() HealingCoordinatorMetrics {
	c.metricsMutex.RLock()
	defer c.metricsMutex.RUnlock()

	metrics := c.metrics
	metrics.QueuedTasks = c.taskQueue.Size()
	metrics.UptimeSeconds = int64(time.Since(c.startTime).Seconds())

	// Add LLM metrics if analyzer is available
	if c.llmAnalyzer != nil {
		if llmMetrics := c.llmAnalyzer.GetCostMetrics(); llmMetrics != nil {
			metrics.TotalLLMCalls = llmMetrics.TotalCalls
			metrics.TotalLLMTokens = llmMetrics.TotalInputTokens + llmMetrics.TotalOutputTokens
			metrics.TotalLLMCost = llmMetrics.TotalCost
			metrics.LLMCacheHitRate = llmMetrics.CacheHitRate

			// Calculate average cost per heal
			if metrics.CompletedTasks > 0 {
				metrics.AverageLLMCostPerHeal = metrics.TotalLLMCost / float64(metrics.CompletedTasks)
			}
		}
	}

	return metrics
}

// SetLLMAnalyzer sets the LLM analyzer for cost tracking
func (c *HealingCoordinator) SetLLMAnalyzer(analyzer *EnhancedLLMAnalyzer) {
	c.llmAnalyzer = analyzer
	// Connect the metrics exporter to the LLM analyzer
	if analyzer != nil && c.metricsExporter != nil {
		analyzer.SetMetricsExporter(c.metricsExporter)
	}
}

// GetLLMCostForTransformation returns the total LLM cost for a specific transformation
func (c *HealingCoordinator) GetLLMCostForTransformation(transformID string) float64 {
	if c.llmAnalyzer != nil {
		return c.llmAnalyzer.GetTransformationLLMCost(transformID)
	}
	return 0
}

// IsRunning returns true if the coordinator is running
func (c *HealingCoordinator) IsRunning() bool {
	return atomic.LoadInt32(&c.running) == 1
}

// GetActiveTaskCount returns the number of currently active tasks
func (c *HealingCoordinator) GetActiveTaskCount() int {
	count := 0
	c.activeTasks.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetMetricsExporter returns the Prometheus metrics exporter
func (c *HealingCoordinator) GetMetricsExporter() *HealingMetricsExporter {
	return c.metricsExporter
}

// GetActiveAlerts returns currently active alerts
func (c *HealingCoordinator) GetActiveAlerts() []*HealingAlert {
	if c.alertManager != nil {
		return c.alertManager.GetActiveAlerts()
	}
	return nil
}

// GetAlertHistory returns the alert history
func (c *HealingCoordinator) GetAlertHistory() []*HealingAlert {
	if c.alertManager != nil {
		return c.alertManager.GetAlertHistory()
	}
	return nil
}

// Metrics update methods
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

// TaskQueue implementation with priority support

// PushTask adds a task to the queue
func (tq *TaskQueue) PushTask(task *HealingTask) {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	heap.Push(tq, task)
}

// PopTask removes and returns the highest priority task
func (tq *TaskQueue) PopTask() *HealingTask {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	if len(tq.items) == 0 {
		return nil
	}

	return heap.Pop(tq).(*HealingTask)
}

// Size returns the queue length (with lock for external access)
func (tq *TaskQueue) Size() int {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()
	return len(tq.items)
}

// Heap interface implementation for TaskQueue (methods called by heap package)

// Len implements heap.Interface (assumes caller holds appropriate locks)
func (tq *TaskQueue) Len() int {
	return len(tq.items)
}

// Less implements heap.Interface (lower priority value = higher priority)
func (tq *TaskQueue) Less(i, j int) bool {
	// Lower priority value means higher priority
	if tq.items[i].Priority != tq.items[j].Priority {
		return tq.items[i].Priority < tq.items[j].Priority
	}

	// If same priority, use submission time (FIFO)
	return tq.items[i].SubmittedAt.Before(tq.items[j].SubmittedAt)
}

// Swap implements heap.Interface
func (tq *TaskQueue) Swap(i, j int) {
	tq.items[i], tq.items[j] = tq.items[j], tq.items[i]
}

// Push implements heap.Interface
func (tq *TaskQueue) Push(x interface{}) {
	tq.items = append(tq.items, x.(*HealingTask))
}

// Pop implements heap.Interface
func (tq *TaskQueue) Pop() interface{} {
	old := tq.items
	n := len(old)
	item := old[n-1]
	tq.items = old[0 : n-1]
	return item
}
