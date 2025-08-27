package jobs

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/storage"
)

// Manager manages asynchronous OpenRewrite jobs
type Manager struct {
	executor     *executor.Executor
	storage      *storage.StorageClient
	workers      chan struct{}          // Worker pool semaphore
	cancelMap    map[string]context.CancelFunc
	cancelMapMu  sync.RWMutex
	jobQueue     chan JobWork
	maxJobs      int
}

// JobWork represents work to be done by a worker
type JobWork struct {
	JobID   string
	Request executor.TransformRequest
	Context context.Context
	Cancel  context.CancelFunc
}

// NewManager creates a new job manager
func NewManager(exec *executor.Executor, storageClient *storage.StorageClient) *Manager {
	// Get configuration from environment
	workerPoolSize := getEnvInt("WORKER_POOL_SIZE", 2)
	maxJobs := getEnvInt("MAX_CONCURRENT_JOBS", 5)
	
	manager := &Manager{
		executor:    exec,
		storage:     storageClient,
		workers:     make(chan struct{}, workerPoolSize),
		cancelMap:   make(map[string]context.CancelFunc),
		jobQueue:    make(chan JobWork, maxJobs),
		maxJobs:     maxJobs,
	}
	
	// Start worker goroutines
	for i := 0; i < workerPoolSize; i++ {
		go manager.worker(i)
	}
	
	log.Printf("[JobManager] Started with %d workers, max %d concurrent jobs", workerPoolSize, maxJobs)
	return manager
}

// CreateJob creates a new asynchronous job
func (m *Manager) CreateJob(request CreateJobRequest) (string, error) {
	// Generate job ID
	jobID := uuid.New().String()
	
	// Decode archive to get size for validation
	archiveData, err := base64.StdEncoding.DecodeString(request.TarArchive)
	if err != nil {
		return "", fmt.Errorf("invalid base64 archive: %w", err)
	}
	
	// Create job metadata in storage
	_, err = m.storage.CreateJob(jobID, request.RecipeConfig.Recipe, archiveData)
	if err != nil {
		return "", fmt.Errorf("failed to create job metadata: %w", err)
	}
	
	// Create transform request
	transformRequest := executor.TransformRequest{
		JobID:        jobID,
		TarArchive:   request.TarArchive,
		RecipeConfig: request.RecipeConfig,
	}
	
	// Create context with cancel function
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	
	// Store cancel function for job cancellation
	m.cancelMapMu.Lock()
	m.cancelMap[jobID] = cancel
	m.cancelMapMu.Unlock()
	
	// Queue job for execution
	work := JobWork{
		JobID:   jobID,
		Request: transformRequest,
		Context: ctx,
		Cancel:  cancel,
	}
	
	select {
	case m.jobQueue <- work:
		log.Printf("[JobManager] Queued job %s for execution", jobID)
	default:
		// Queue is full
		cancel()
		m.cancelMapMu.Lock()
		delete(m.cancelMap, jobID)
		m.cancelMapMu.Unlock()
		
		// Mark job as failed
		m.storage.FailJob(jobID, "Job queue is full, please try again later")
		return "", fmt.Errorf("job queue is full (max %d concurrent jobs)", m.maxJobs)
	}
	
	log.Printf("[JobManager] Created job %s (recipe: %s, archive size: %d bytes)", 
		jobID, request.RecipeConfig.Recipe, len(archiveData))
	
	return jobID, nil
}

// CancelJob cancels a running job
func (m *Manager) CancelJob(jobID string) error {
	// Check if job exists
	job, err := m.storage.GetJob(jobID)
	if err != nil {
		return err
	}
	
	// Check if job can be cancelled
	if job.Status == "completed" || job.Status == "failed" {
		return fmt.Errorf("job %s is already %s and cannot be cancelled", jobID, job.Status)
	}
	
	// Cancel the job context
	m.cancelMapMu.Lock()
	cancel, exists := m.cancelMap[jobID]
	if exists {
		cancel()
		delete(m.cancelMap, jobID)
	}
	m.cancelMapMu.Unlock()
	
	// Update job status
	if err := m.storage.FailJob(jobID, "Job cancelled by user"); err != nil {
		log.Printf("[JobManager] Failed to update cancelled job status: %v", err)
	}
	
	log.Printf("[JobManager] Cancelled job %s", jobID)
	return nil
}

// worker processes jobs from the queue
func (m *Manager) worker(workerID int) {
	log.Printf("[Worker-%d] Started", workerID)
	
	for work := range m.jobQueue {
		log.Printf("[Worker-%d] Processing job %s", workerID, work.JobID)
		
		// Acquire worker slot
		m.workers <- struct{}{}
		
		// Update job status to running
		if err := m.storage.UpdateJobStatus(work.JobID, "running", 10); err != nil {
			log.Printf("[Worker-%d] Failed to update job status: %v", workerID, err)
		}
		
		// Execute transformation
		result, err := m.executor.ExecuteTransformation(work.Context, work.Request)
		
		if err != nil {
			log.Printf("[Worker-%d] Job %s failed: %v", workerID, work.JobID, err)
			m.storage.FailJob(work.JobID, err.Error())
		} else if !result.Success {
			log.Printf("[Worker-%d] Job %s completed with errors", workerID, work.JobID)
			errorMsg := "Transformation completed with errors"
			if len(result.Errors) > 0 {
				errorMsg = result.Errors[0].Message
			}
			m.storage.FailJob(work.JobID, errorMsg)
		} else {
			log.Printf("[Worker-%d] Job %s completed successfully (changes: %d)", 
				workerID, work.JobID, result.ChangesApplied)
			// Store diff and mark as completed
			if err := m.storage.CompleteJob(work.JobID, []byte(result.Diff)); err != nil {
				log.Printf("[Worker-%d] Failed to complete job %s: %v", workerID, work.JobID, err)
				m.storage.FailJob(work.JobID, "Failed to store results: "+err.Error())
			}
		}
		
		// Clean up cancel function
		m.cancelMapMu.Lock()
		if cancel, exists := m.cancelMap[work.JobID]; exists {
			cancel() // Clean up context
			delete(m.cancelMap, work.JobID)
		}
		m.cancelMapMu.Unlock()
		
		// Release worker slot
		<-m.workers
		
		log.Printf("[Worker-%d] Finished processing job %s", workerID, work.JobID)
	}
	
	log.Printf("[Worker-%d] Stopped", workerID)
}

// GetQueueSize returns the current size of the job queue
func (m *Manager) GetQueueSize() int {
	return len(m.jobQueue)
}

// GetActiveWorkers returns the number of currently active workers
func (m *Manager) GetActiveWorkers() int {
	return len(m.workers)
}

// Shutdown gracefully shuts down the job manager
func (m *Manager) Shutdown() {
	log.Printf("[JobManager] Shutting down...")
	
	// Cancel all running jobs
	m.cancelMapMu.Lock()
	for jobID, cancel := range m.cancelMap {
		log.Printf("[JobManager] Cancelling job %s during shutdown", jobID)
		cancel()
		m.storage.FailJob(jobID, "Service is shutting down")
	}
	m.cancelMap = make(map[string]context.CancelFunc)
	m.cancelMapMu.Unlock()
	
	// Close job queue
	close(m.jobQueue)
	
	log.Printf("[JobManager] Shutdown complete")
}

// Helper function to get environment variable as int with default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}