package arf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

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
