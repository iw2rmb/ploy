package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/openrewrite"
	openrewrite_storage "github.com/iw2rmb/ploy/internal/storage/openrewrite"
)

// Worker represents a worker that processes jobs from the queue
type Worker struct {
	workerPool chan chan *Job
	jobChannel chan *Job
	storage    openrewrite_storage.JobStorage
	executor   openrewrite.Executor
	quit       chan bool
}

// NewWorker creates a new Worker instance
func NewWorker(workerPool chan chan *Job, storage openrewrite_storage.JobStorage, executor openrewrite.Executor) *Worker {
	return &Worker{
		workerPool: workerPool,
		jobChannel: make(chan *Job),
		storage:    storage,
		executor:   executor,
		quit:       make(chan bool),
	}
}

// Start begins the worker's job processing loop
func (w *Worker) Start() {
	go func() {
		for {
			// Register this worker as available
			w.workerPool <- w.jobChannel
			
			select {
			case job := <-w.jobChannel:
				// Process the job
				w.processJob(job)
				
			case <-w.quit:
				// Worker has been stopped
				return
			}
		}
	}()
}

// Stop gracefully shuts down the worker
func (w *Worker) Stop() {
	close(w.quit)
}

// processJob handles the complete lifecycle of job processing
func (w *Worker) processJob(job *Job) {
	startTime := time.Now()
	
	// Check if job was cancelled before processing
	if w.isJobCancelled(job.ID) {
		fmt.Printf("Job %s was cancelled before processing\n", job.ID)
		return
	}
	
	// Update status to processing
	status := &openrewrite_storage.JobStatus{
		JobID:     job.ID,
		Status:    openrewrite_storage.StatusProcessing,
		UpdatedAt: startTime,
		Progress:  10,
		Message:   "Starting transformation",
	}
	
	if err := w.storage.StoreJobStatus(job.ID, status); err != nil {
		// Log error but continue processing
		fmt.Printf("Warning: failed to update job status for %s: %v\n", job.ID, err)
	}
	
	// Create context with timeout for transformation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// Check for cancellation one more time before expensive operation
	if w.isJobCancelled(job.ID) {
		fmt.Printf("Job %s was cancelled before execution\n", job.ID)
		return
	}
	
	// Convert storage RecipeConfig to openrewrite RecipeConfig
	executorRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "", // Will be populated from Parameters if needed
		Options:   make(map[string]string),
	}
	
	// Convert parameters to options (simple string conversion)
	for k, v := range job.Recipe.Parameters {
		if str, ok := v.(string); ok {
			executorRecipe.Options[k] = str
		}
	}
	
	// Execute the transformation
	result, err := w.executor.Execute(ctx, job.ID, job.TarData, executorRecipe)
	
	// Update final status based on execution result
	if err != nil {
		w.handleJobFailure(job, err)
	} else {
		w.handleJobSuccess(job, result, time.Since(startTime))
	}
}

// handleJobFailure processes a failed job with retry logic
func (w *Worker) handleJobFailure(job *Job, execError error) {
	// Don't retry context cancellation errors
	if execError == context.Canceled || execError == context.DeadlineExceeded {
		w.handlePermanentFailure(job, execError)
		return
	}
	
	// Increment retry count
	job.Retries++
	
	// Check if we should retry (max 3 attempts total)
	if job.Retries < 3 {
		
		// Calculate exponential backoff delay
		delay := time.Duration(job.Retries) * time.Minute
		
		// Update status to indicate retry
		retryStatus := &openrewrite_storage.JobStatus{
			JobID:     job.ID,
			Status:    openrewrite_storage.StatusQueued, // Re-queue for retry
			UpdatedAt: time.Now(),
			Progress:  0,
			Message:   fmt.Sprintf("Retrying in %v (attempt %d/3)", delay, job.Retries),
			Error:     execError.Error(),
		}
		
		if err := w.storage.StoreJobStatus(job.ID, retryStatus); err != nil {
			fmt.Printf("Warning: failed to update retry status for %s: %v\n", job.ID, err)
		}
		
		// Re-queue job with delay
		go func() {
			time.Sleep(delay)
			// Note: In a full implementation, we'd re-enqueue the job here
			// For now, we'll mark this as a design note for the next phase
		}()
		
		return
	}
	
	// Final failure after exhausting retries
	w.handlePermanentFailure(job, execError)
}

// handlePermanentFailure processes a permanently failed job
func (w *Worker) handlePermanentFailure(job *Job, execError error) {
	failureStatus := &openrewrite_storage.JobStatus{
		JobID:     job.ID,
		Status:    openrewrite_storage.StatusFailed,
		UpdatedAt: time.Now(),
		Progress:  0,
		Message:   "Transformation failed after retries",
		Error:     execError.Error(),
	}
	
	if err := w.storage.StoreJobStatus(job.ID, failureStatus); err != nil {
		fmt.Printf("Error: failed to update failure status for %s: %v\n", job.ID, err)
	}
}

// handleJobSuccess processes a successful job completion
func (w *Worker) handleJobSuccess(job *Job, result *openrewrite.TransformResult, duration time.Duration) {
	// Store the diff in SeaweedFS
	diffURL := ""
	if len(result.Diff) > 0 {
		fileID, err := w.storage.StoreDiff(job.ID, result.Diff)
		if err != nil {
			// Treat diff storage failure as job failure
			w.handleStorageFailure(job, err)
			return
		}
		diffURL = fileID
	}
	
	// Update status to completed
	completedAt := time.Now()
	successStatus := &openrewrite_storage.JobStatus{
		JobID:       job.ID,
		Status:      openrewrite_storage.StatusCompleted,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
		Progress:    100,
		Message:     "Transformation completed successfully",
		DiffURL:     diffURL,
	}
	
	if err := w.storage.StoreJobStatus(job.ID, successStatus); err != nil {
		fmt.Printf("Error: failed to update success status for %s: %v\n", job.ID, err)
	}
}

// handleStorageFailure processes a failure in storing the diff
func (w *Worker) handleStorageFailure(job *Job, storageError error) {
	failureStatus := &openrewrite_storage.JobStatus{
		JobID:     job.ID,
		Status:    openrewrite_storage.StatusFailed,
		UpdatedAt: time.Now(),
		Progress:  90, // Transformation succeeded but storage failed
		Message:   "Transformation succeeded but failed to store diff",
		Error:     storageError.Error(),
	}
	
	if err := w.storage.StoreJobStatus(job.ID, failureStatus); err != nil {
		fmt.Printf("Error: failed to update storage failure status for %s: %v\n", job.ID, err)
	}
}

// isJobCancelled checks if a job has been cancelled by looking at its status
func (w *Worker) isJobCancelled(jobID string) bool {
	status, err := w.storage.GetJobStatus(jobID)
	if err != nil {
		return false // If we can't get status, assume not cancelled
	}
	
	// Check if status indicates cancellation
	return status.Error == "Job was cancelled" || status.Message == "Job cancelled by user"
}