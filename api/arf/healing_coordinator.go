package arf

import (
	"container/heap"
	"context"
	"fmt"
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

	// Metrics
	metrics      HealingCoordinatorMetrics
	metricsMutex sync.RWMutex
	startTime    time.Time

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

	return &HealingCoordinator{
		config:           config,
		semaphore:        make(chan struct{}, config.MaxParallelAttempts),
		taskQueue:        &TaskQueue{items: make([]*HealingTask, 0, queueSize)},
		taskSubmissions:  make(chan *HealingTask, queueSize),
		shutdownComplete: make(chan struct{}),
		startTime:        time.Now(),
	}
}

// Start begins the coordinator's operation
func (c *HealingCoordinator) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return fmt.Errorf("coordinator already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.startTime = time.Now()

	// Start the main processing loop
	c.wg.Add(1)
	go c.processLoop()

	return nil
}

// Stop gracefully shuts down the coordinator
func (c *HealingCoordinator) Stop() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return // Not running
	}

	// Cancel context to stop accepting new tasks
	c.cancel()

	// Wait for all workers to complete
	c.wg.Wait()

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

	// Validate task
	if err := c.validateTask(task); err != nil {
		c.incrementRejectedTasks()
		return fmt.Errorf("task validation failed: %w", err)
	}

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
		return fmt.Errorf("max healing depth (%d) exceeded: %d", c.config.MaxHealingDepth, depth)
	}

	// Check total attempts limit
	c.metricsMutex.RLock()
	totalAttempts := c.metrics.TotalSubmitted + 1
	c.metricsMutex.RUnlock()

	if totalAttempts > c.config.MaxTotalAttempts {
		return fmt.Errorf("max total attempts (%d) exceeded", c.config.MaxTotalAttempts)
	}

	return nil
}

// processLoop is the main processing loop for the coordinator
func (c *HealingCoordinator) processLoop() {
	defer c.wg.Done()

	for {
		select {
		case task := <-c.taskSubmissions:
			c.enqueueTask(task)
			c.tryStartWorker()

		case <-c.ctx.Done():
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
	}()

	// Track active task
	c.activeTasks.Store(task.TransformID, task)
	c.incrementActiveWorkers()

	// Create task-specific context with timeout
	taskCtx, cancel := context.WithTimeout(c.ctx, c.config.AttemptTimeout)
	defer cancel()

	// Execute the task
	if err := task.ExecuteFn(taskCtx); err != nil {
		c.incrementFailedTasks()
	} else {
		c.incrementCompletedTasks()
	}
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

	return metrics
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

// Metrics update methods
func (c *HealingCoordinator) incrementActiveWorkers() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.ActiveWorkers++
}

func (c *HealingCoordinator) decrementActiveWorkers() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.ActiveWorkers--
}

func (c *HealingCoordinator) incrementQueuedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.QueuedTasks++
}

func (c *HealingCoordinator) decrementQueuedTasks() {
	c.metricsMutex.Lock()
	defer c.metricsMutex.Unlock()
	c.metrics.QueuedTasks--
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
