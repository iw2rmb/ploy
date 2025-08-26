package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/openrewrite"
	openrewrite_storage "github.com/iw2rmb/ploy/internal/storage/openrewrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)


func TestNewWorker(t *testing.T) {
	workerPool := make(chan chan *Job, 5)
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)

	worker := NewWorker(workerPool, mockStorage, mockExecutor)
	
	assert.NotNil(t, worker)
	assert.Equal(t, workerPool, worker.workerPool)
	assert.Equal(t, mockStorage, worker.storage)
	assert.Equal(t, mockExecutor, worker.executor)
	assert.NotNil(t, worker.jobChannel)
	assert.NotNil(t, worker.quit)
}

func TestWorker_Start(t *testing.T) {
	workerPool := make(chan chan *Job, 1)
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)

	worker := NewWorker(workerPool, mockStorage, mockExecutor)
	
	// Start worker
	worker.Start()
	
	// Worker should register itself in the pool
	select {
	case jobChan := <-workerPool:
		assert.Equal(t, worker.jobChannel, jobChan)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Worker did not register in pool within timeout")
	}
	
	// Cleanup
	worker.Stop()
}

func TestWorker_Stop(t *testing.T) {
	workerPool := make(chan chan *Job, 1)
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)

	worker := NewWorker(workerPool, mockStorage, mockExecutor)
	worker.Start()
	
	// Wait for worker to register once to ensure it's running
	select {
	case <-workerPool:
		// Worker has registered, now stop it
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Worker did not register within timeout")
	}
	
	// Stop should shut down the worker gracefully  
	worker.Stop()
	
	// Verify worker is no longer active by checking if it tries to register again
	select {
	case <-workerPool:
		t.Fatal("Worker should not register after being stopped")
	case <-time.After(100 * time.Millisecond):
		// Expected - worker should be stopped
	}
}

func TestWorker_ProcessJob_Success(t *testing.T) {
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		TarData:   []byte("test tar data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
		Retries:   0,
	}
	
	// Expected transformation result
	result := &openrewrite.TransformResult{
		Success:  true,
		Diff:     []byte("diff content"),
		Duration: 2 * time.Second,
	}
	
	// Mock cancellation check - job is not cancelled
	mockStorage.On("GetJobStatus", "test-job-1").Return(&openrewrite_storage.JobStatus{
		JobID:   "test-job-1",
		Status:  openrewrite_storage.StatusQueued,
		Message: "Job queued for processing",
		Error:   "",
	}, nil)
	
	// Mock expectations for processing status
	mockStorage.On("StoreJobStatus", "test-job-1", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusProcessing && status.Progress == 10
	})).Return(nil)
	
	// Mock successful execution with converted recipe config
	expectedRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "",
		Options:   make(map[string]string),
	}
	mockExecutor.On("Execute", mock.Anything, "test-job-1", job.TarData, expectedRecipe).Return(result, nil)
	
	// Mock diff storage
	mockStorage.On("StoreDiff", "test-job-1", result.Diff).Return("file-id-123", nil)
	
	// Mock completion status
	mockStorage.On("StoreJobStatus", "test-job-1", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusCompleted && 
			status.Progress == 100 &&
			status.DiffURL == "file-id-123" &&
			status.CompletedAt != nil
	})).Return(nil)
	
	worker := &Worker{
		storage:  mockStorage,
		executor: mockExecutor,
	}
	
	worker.processJob(job)
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

func TestWorker_ProcessJob_ExecutionErrorWithRetry(t *testing.T) {
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	job := &Job{
		ID:        "test-job-2",
		Priority:  3,
		TarData:   []byte("test tar data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
		Retries:   0,
	}
	
	// Mock cancellation check - job is not cancelled
	mockStorage.On("GetJobStatus", "test-job-2").Return(&openrewrite_storage.JobStatus{
		JobID:   "test-job-2",
		Status:  openrewrite_storage.StatusQueued,
		Message: "Job queued for processing",
		Error:   "",
	}, nil)
	
	// Mock processing status
	mockStorage.On("StoreJobStatus", "test-job-2", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusProcessing
	})).Return(nil)
	
	// Mock execution failure
	execError := errors.New("transformation failed")
	expectedRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "",
		Options:   make(map[string]string),
	}
	mockExecutor.On("Execute", mock.Anything, "test-job-2", job.TarData, expectedRecipe).Return(nil, execError)
	
	// Mock retry status (job should be retried first)
	mockStorage.On("StoreJobStatus", "test-job-2", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusQueued && 
			status.Error == execError.Error() &&
			status.Message == "Retrying in 1m0s (attempt 1/3)"
	})).Return(nil)
	
	// Mock failure status (this test doesn't expect final failure since it's the first retry)
	// But we need to handle the case where we exhaust all retries
	// For this test, we simulate that the job gets queued for retry
	
	worker := &Worker{
		storage:  mockStorage,
		executor: mockExecutor,
	}
	
	worker.processJob(job)
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

func TestWorker_ProcessJob_RetryLogic(t *testing.T) {
	workerPool := make(chan chan *Job, 1)
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	job := &Job{
		ID:        "test-job-retry",
		Priority:  3,
		TarData:   []byte("test tar data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
		Retries:   2, // Already retried twice
	}
	
	// Mock cancellation check - job is not cancelled
	mockStorage.On("GetJobStatus", "test-job-retry").Return(&openrewrite_storage.JobStatus{
		JobID:   "test-job-retry",
		Status:  openrewrite_storage.StatusQueued,
		Message: "Job queued for processing",
		Error:   "",
	}, nil)
	
	// Mock processing status
	mockStorage.On("StoreJobStatus", "test-job-retry", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusProcessing
	})).Return(nil)
	
	// Mock execution failure (3rd retry - should fail permanently)
	execError := errors.New("persistent failure")
	expectedRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "",
		Options:   make(map[string]string),
	}
	mockExecutor.On("Execute", mock.Anything, "test-job-retry", job.TarData, expectedRecipe).Return(nil, execError)
	
	// Mock final failure status (job has already been retried 2 times, this is the 3rd failure)
	mockStorage.On("StoreJobStatus", "test-job-retry", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusFailed && 
			status.Error == execError.Error() &&
			status.Message == "Transformation failed after retries"
	})).Return(nil)
	
	worker := &Worker{
		workerPool: workerPool,
		storage:    mockStorage,
		executor:   mockExecutor,
	}
	
	worker.processJob(job)
	
	assert.Equal(t, 3, job.Retries, "Job should have been marked with final retry count")
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

func TestWorker_ProcessJob_DiffStorageError(t *testing.T) {
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	job := &Job{
		ID:        "test-job-3",
		TarData:   []byte("test tar data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
		Retries:   0,
	}
	
	// Successful transformation
	result := &openrewrite.TransformResult{
		Success: true,
		Diff:    []byte("diff content"),
	}
	
	// Mock cancellation check - job is not cancelled
	mockStorage.On("GetJobStatus", "test-job-3").Return(&openrewrite_storage.JobStatus{
		JobID:   "test-job-3",
		Status:  openrewrite_storage.StatusQueued,
		Message: "Job queued for processing",
		Error:   "",
	}, nil)
	
	// Mock processing status
	mockStorage.On("StoreJobStatus", "test-job-3", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusProcessing
	})).Return(nil)
	
	// Mock successful execution
	expectedRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "",
		Options:   make(map[string]string),
	}
	mockExecutor.On("Execute", mock.Anything, "test-job-3", job.TarData, expectedRecipe).Return(result, nil)
	
	// Mock diff storage failure
	storageError := errors.New("storage unavailable")
	mockStorage.On("StoreDiff", "test-job-3", result.Diff).Return("", storageError)
	
	// Mock failure status due to storage error
	mockStorage.On("StoreJobStatus", "test-job-3", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusFailed && 
			status.Error == storageError.Error()
	})).Return(nil)
	
	worker := &Worker{
		storage:  mockStorage,
		executor: mockExecutor,
	}
	
	worker.processJob(job)
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

func TestWorker_ConcurrentJobProcessing(t *testing.T) {
	workerPool := make(chan chan *Job, 1)
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	// Create one worker for simpler testing
	worker := NewWorker(workerPool, mockStorage, mockExecutor)
	worker.Start()
	defer worker.Stop()
	
	// Test with a smaller number of jobs for more reliable testing
	numJobs := 3
	processedJobs := make([]string, 0, numJobs)
	var mu sync.Mutex
	
	// Mock all expected calls - each job will have processing status + completion status
	for i := 0; i < numJobs; i++ {
		jobID := fmt.Sprintf("concurrent-job-%d", i)
		
		result := &openrewrite.TransformResult{
			Success: true,
			Diff:    []byte(fmt.Sprintf("diff-%d", i)),
		}
		
		// Mock cancellation check - job is not cancelled
		mockStorage.On("GetJobStatus", jobID).Return(&openrewrite_storage.JobStatus{
			JobID:   jobID,
			Status:  openrewrite_storage.StatusQueued,
			Message: "Job queued for processing",
			Error:   "",
		}, nil)
		
		// Processing status call
		mockStorage.On("StoreJobStatus", jobID, mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
			return status.Status == openrewrite_storage.StatusProcessing
		})).Return(nil)
		
		// Execute call with expected recipe conversion
		expectedRecipe := openrewrite.RecipeConfig{
			Recipe:    fmt.Sprintf("recipe-%d", i),
			Artifacts: "",
			Options:   make(map[string]string),
		}
		mockExecutor.On("Execute", mock.Anything, jobID, mock.Anything, expectedRecipe).Return(result, nil)
		
		// Diff storage call
		mockStorage.On("StoreDiff", jobID, mock.Anything).Return(fmt.Sprintf("file-%d", i), nil)
		
		// Completion status call
		mockStorage.On("StoreJobStatus", jobID, mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
			return status.Status == openrewrite_storage.StatusCompleted
		})).Return(nil)
	}
	
	// Process jobs sequentially but through the worker pool
	for i := 0; i < numJobs; i++ {
		jobID := fmt.Sprintf("concurrent-job-%d", i)
		job := &Job{
			ID:        jobID,
			TarData:   []byte(fmt.Sprintf("data-%d", i)),
			Recipe:    openrewrite_storage.RecipeConfig{Name: fmt.Sprintf("recipe-%d", i)},
			CreatedAt: time.Now(),
			Retries:   0,
		}
		
		// Get available worker and send job
		workerChan := <-workerPool
		workerChan <- job
		
		// Track processed job
		mu.Lock()
		processedJobs = append(processedJobs, jobID)
		mu.Unlock()
		
		// Give time for processing to complete
		time.Sleep(10 * time.Millisecond)
	}
	
	assert.Equal(t, numJobs, len(processedJobs), "All jobs should be sent to workers")
	
	// Wait a bit more for all processing to complete
	time.Sleep(100 * time.Millisecond)
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

func TestWorker_ContextCancellation(t *testing.T) {
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	job := &Job{
		ID:        "cancel-test",
		TarData:   []byte("test data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
		Retries:   0,
	}
	
	// Mock cancellation check - job is not cancelled
	mockStorage.On("GetJobStatus", "cancel-test").Return(&openrewrite_storage.JobStatus{
		JobID:   "cancel-test",
		Status:  openrewrite_storage.StatusQueued,
		Message: "Job queued for processing",
		Error:   "",
	}, nil)
	
	// Mock processing status
	mockStorage.On("StoreJobStatus", "cancel-test", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusProcessing
	})).Return(nil)
	
	// Mock context cancellation during execution
	cancelError := context.Canceled
	expectedRecipe := openrewrite.RecipeConfig{
		Recipe:    job.Recipe.Name,
		Artifacts: "",
		Options:   make(map[string]string),
	}
	mockExecutor.On("Execute", mock.Anything, "cancel-test", job.TarData, expectedRecipe).Return(nil, cancelError)
	
	// Mock cancellation status
	mockStorage.On("StoreJobStatus", "cancel-test", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.Status == openrewrite_storage.StatusFailed && 
			status.Error == cancelError.Error()
	})).Return(nil)
	
	worker := &Worker{
		storage:  mockStorage,
		executor: mockExecutor,
	}
	
	worker.processJob(job)
	
	mockStorage.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

