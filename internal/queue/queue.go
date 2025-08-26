package queue

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/openrewrite"
	openrewrite_storage "github.com/iw2rmb/ploy/internal/storage/openrewrite"
)

// JobQueue manages a priority queue of transformation jobs
type JobQueue struct {
	mu            sync.RWMutex
	jobs          JobHeap
	maxWorkers    int
	storage       openrewrite_storage.JobStorage
	executor      openrewrite.Executor
	workers       chan chan *Job
	workerList    []*Worker
	running       bool
	paused        bool
	quit          chan bool
	cancelledJobs map[string]bool // Track cancelled jobs
}

// NewJobQueue creates a new JobQueue instance
func NewJobQueue(maxWorkers int, storage openrewrite_storage.JobStorage, executor openrewrite.Executor) *JobQueue {
	q := &JobQueue{
		jobs:          make(JobHeap, 0),
		maxWorkers:    maxWorkers,
		storage:       storage,
		executor:      executor,
		workers:       make(chan chan *Job, maxWorkers),
		workerList:    make([]*Worker, 0, maxWorkers),
		running:       false,
		paused:        false,
		quit:          make(chan bool),
		cancelledJobs: make(map[string]bool),
	}
	
	heap.Init(&q.jobs)
	return q
}

// Enqueue adds a job to the queue with proper priority ordering
func (q *JobQueue) Enqueue(job *Job) error {
	// Store initial status in storage backend
	status := &openrewrite_storage.JobStatus{
		JobID:     job.ID,
		Status:    openrewrite_storage.StatusQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Progress:  0,
		Message:   "Job queued for processing",
	}
	
	if err := q.storage.StoreJobStatus(job.ID, status); err != nil {
		return fmt.Errorf("failed to store job status: %w", err)
	}
	
	// Add to priority queue
	q.mu.Lock()
	defer q.mu.Unlock()
	
	heap.Push(&q.jobs, job)
	return nil
}

// Dequeue removes and returns the highest priority job from the queue
func (q *JobQueue) Dequeue() (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.jobs.Len() == 0 {
		return nil, fmt.Errorf("queue is empty")
	}
	
	job := heap.Pop(&q.jobs).(*Job)
	return job, nil
}

// Peek returns the highest priority job without removing it
func (q *JobQueue) Peek() (*Job, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	if q.jobs.Len() == 0 {
		return nil, fmt.Errorf("queue is empty")
	}
	
	// Return a copy to prevent external modification
	topJob := q.jobs[0]
	jobCopy := &Job{
		ID:        topJob.ID,
		Priority:  topJob.Priority,
		TarData:   topJob.TarData,
		Recipe:    topJob.Recipe,
		CreatedAt: topJob.CreatedAt,
		Retries:   topJob.Retries,
		index:     topJob.index,
	}
	
	return jobCopy, nil
}

// Size returns the number of jobs currently in the queue
func (q *JobQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	return q.jobs.Len()
}

// IsEmpty returns true if the queue has no jobs
func (q *JobQueue) IsEmpty() bool {
	return q.Size() == 0
}

// UpdateMetrics stores current queue metrics to the storage backend
func (q *JobQueue) UpdateMetrics() error {
	q.mu.RLock()
	queueDepth := q.jobs.Len()
	activeWorkers := len(q.workerList)
	cancelledCount := len(q.cancelledJobs)
	q.mu.RUnlock()
	
	metrics := &openrewrite_storage.Metrics{
		QueueDepth:    queueDepth,
		ActiveWorkers: activeWorkers,
		LastActivity:  time.Now(),
		JobsProcessed: 0, // Could be enhanced to track this
		JobsFailed:    int64(cancelledCount), // Use cancelled count as failed for now
	}
	
	return q.storage.StoreMetrics(metrics)
}

// Clear removes all jobs from the queue
func (q *JobQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	q.jobs = make(JobHeap, 0)
	heap.Init(&q.jobs)
}

// Contains checks if a job with the given ID exists in the queue
func (q *JobQueue) Contains(jobID string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	for _, job := range q.jobs {
		if job.ID == jobID {
			return true
		}
	}
	return false
}

// RemoveJob removes a specific job from the queue by ID
func (q *JobQueue) RemoveJob(jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	for i, job := range q.jobs {
		if job.ID == jobID {
			heap.Remove(&q.jobs, i)
			return nil
		}
	}
	
	return fmt.Errorf("job not found: %s", jobID)
}

// Start initializes the worker pool and job dispatcher
func (q *JobQueue) Start() {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.running {
		return // Already running
	}
	
	q.running = true
	q.startWorkers()
	q.startDispatcher()
}

// Stop gracefully shuts down the worker pool and dispatcher
func (q *JobQueue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if !q.running {
		return // Already stopped
	}
	
	q.running = false
	close(q.quit)
	
	// Stop all workers
	for _, worker := range q.workerList {
		worker.Stop()
	}
	
	q.workerList = q.workerList[:0] // Clear worker list
}

// Drain pauses the queue and waits for current jobs to complete
func (q *JobQueue) Drain() {
	q.Pause()
	// Give time for current jobs to complete
	// In a production system, this would be more sophisticated
	time.Sleep(1 * time.Second)
}

// startWorkers creates and starts the worker pool
func (q *JobQueue) startWorkers() {
	for i := 0; i < q.maxWorkers; i++ {
		worker := NewWorker(q.workers, q.storage, q.executor)
		worker.Start()
		q.workerList = append(q.workerList, worker)
	}
}

// startDispatcher starts the job dispatcher goroutine
func (q *JobQueue) startDispatcher() {
	go func() {
		for {
			select {
			case <-q.quit:
				return
			default:
				q.dispatchNextJob()
			}
		}
	}()
}

// dispatchNextJob gets the next job from the queue and assigns it to an available worker
func (q *JobQueue) dispatchNextJob() {
	q.mu.Lock()
	
	// Check if paused
	if q.paused {
		q.mu.Unlock()
		time.Sleep(100 * time.Millisecond) // Paused, wait a bit
		return
	}
	
	if q.jobs.Len() == 0 {
		q.mu.Unlock()
		time.Sleep(100 * time.Millisecond) // No jobs, wait a bit
		return
	}
	
	// Get the highest priority job
	job := heap.Pop(&q.jobs).(*Job)
	
	// Check if job was cancelled
	if q.cancelledJobs[job.ID] {
		q.mu.Unlock()
		return // Skip cancelled jobs
	}
	
	q.mu.Unlock()
	
	// Wait for an available worker and dispatch the job
	select {
	case workerChan := <-q.workers:
		workerChan <- job
	case <-q.quit:
		// If we're shutting down, put the job back
		q.mu.Lock()
		heap.Push(&q.jobs, job)
		q.mu.Unlock()
		return
	}
}

// CancelJob marks a job as cancelled and removes it from the queue if not yet processed
func (q *JobQueue) CancelJob(jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	// Mark job as cancelled
	q.cancelledJobs[jobID] = true
	
	// Try to remove from queue if still queued
	for i, job := range q.jobs {
		if job.ID == jobID {
			heap.Remove(&q.jobs, i)
			break
		}
	}
	
	// Update status to cancelled
	status := &openrewrite_storage.JobStatus{
		JobID:     jobID,
		Status:    openrewrite_storage.StatusFailed,
		UpdatedAt: time.Now(),
		Progress:  0,
		Message:   "Job cancelled by user",
		Error:     "Job was cancelled",
	}
	
	return q.storage.StoreJobStatus(jobID, status)
}

// IsCancelled checks if a job has been cancelled
func (q *JobQueue) IsCancelled(jobID string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.cancelledJobs[jobID]
}

// Pause suspends job processing without stopping workers
func (q *JobQueue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = true
}

// Resume continues job processing
func (q *JobQueue) Resume() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = false
}

// IsPaused returns true if the queue is paused
func (q *JobQueue) IsPaused() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.paused
}

// IsRunning returns true if the worker pool is currently running
func (q *JobQueue) IsRunning() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.running
}

// GetActiveWorkers returns the number of active workers
func (q *JobQueue) GetActiveWorkers() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.workerList)
}