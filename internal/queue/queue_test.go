package queue

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/openrewrite"
	openrewrite_storage "github.com/iw2rmb/ploy/internal/storage/openrewrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockExecutor mocks the OpenRewrite Executor interface
type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) Execute(ctx context.Context, jobID string, tarData []byte, recipe openrewrite.RecipeConfig) (*openrewrite.TransformResult, error) {
	args := m.Called(ctx, jobID, tarData, recipe)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openrewrite.TransformResult), args.Error(1)
}

func (m *MockExecutor) DetectBuildSystem(srcPath string) openrewrite.BuildSystem {
	args := m.Called(srcPath)
	return args.Get(0).(openrewrite.BuildSystem)
}

func (m *MockExecutor) DetectJavaVersion(srcPath string) (openrewrite.JavaVersion, error) {
	args := m.Called(srcPath)
	return args.Get(0).(openrewrite.JavaVersion), args.Error(1)
}

// MockJobStorage mocks the JobStorage interface
type MockJobStorage struct {
	mock.Mock
}

func (m *MockJobStorage) StoreJobStatus(jobID string, status *openrewrite_storage.JobStatus) error {
	args := m.Called(jobID, status)
	return args.Error(0)
}

func (m *MockJobStorage) GetJobStatus(jobID string) (*openrewrite_storage.JobStatus, error) {
	args := m.Called(jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openrewrite_storage.JobStatus), args.Error(1)
}

func (m *MockJobStorage) WatchJobStatus(jobID string, index uint64) (*openrewrite_storage.JobStatus, uint64, error) {
	args := m.Called(jobID, index)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*openrewrite_storage.JobStatus), args.Get(1).(uint64), args.Error(2)
}

func (m *MockJobStorage) StoreDiff(jobID string, diff []byte) (string, error) {
	args := m.Called(jobID, diff)
	return args.String(0), args.Error(1)
}

func (m *MockJobStorage) RetrieveDiff(fileID string) ([]byte, error) {
	args := m.Called(fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockJobStorage) DeleteDiff(fileID string) error {
	args := m.Called(fileID)
	return args.Error(0)
}

func (m *MockJobStorage) StoreMetrics(metrics *openrewrite_storage.Metrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *MockJobStorage) GetMetrics() (*openrewrite_storage.Metrics, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openrewrite_storage.Metrics), args.Error(1)
}

func TestJobHeap_PriorityOrdering(t *testing.T) {
	h := &JobHeap{}
	heap.Init(h)

	now := time.Now()
	
	// Add jobs with different priorities and timestamps
	job1 := &Job{
		ID:        "job-1",
		Priority:  1,
		CreatedAt: now,
	}
	job2 := &Job{
		ID:        "job-2",
		Priority:  5,
		CreatedAt: now.Add(1 * time.Minute),
	}
	job3 := &Job{
		ID:        "job-3",
		Priority:  5,
		CreatedAt: now.Add(-1 * time.Minute), // Older than job2
	}
	job4 := &Job{
		ID:        "job-4",
		Priority:  3,
		CreatedAt: now,
	}

	heap.Push(h, job1)
	heap.Push(h, job2)
	heap.Push(h, job3)
	heap.Push(h, job4)

	// Pop jobs and verify order
	// Should be: job3 (priority 5, older), job2 (priority 5, newer), job4 (priority 3), job1 (priority 1)
	popped := heap.Pop(h).(*Job)
	assert.Equal(t, "job-3", popped.ID, "First job should be job-3 (highest priority, older)")
	
	popped = heap.Pop(h).(*Job)
	assert.Equal(t, "job-2", popped.ID, "Second job should be job-2 (highest priority, newer)")
	
	popped = heap.Pop(h).(*Job)
	assert.Equal(t, "job-4", popped.ID, "Third job should be job-4 (middle priority)")
	
	popped = heap.Pop(h).(*Job)
	assert.Equal(t, "job-1", popped.ID, "Fourth job should be job-1 (lowest priority)")
	
	assert.Equal(t, 0, h.Len(), "Heap should be empty")
}

func TestJobQueue_NewJobQueue(t *testing.T) {
	mockStorage := new(MockJobStorage)
	mockExecutor := new(MockExecutor)
	
	q := NewJobQueue(5, mockStorage, mockExecutor)
	assert.NotNil(t, q)
	assert.Equal(t, 5, q.maxWorkers)
	assert.Equal(t, 0, q.Size())
	assert.False(t, q.IsRunning())
}

func TestJobQueue_Enqueue(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		TarData:   []byte("test data"),
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	// Expect status to be stored
	mockStorage.On("StoreJobStatus", "test-job-1", mock.MatchedBy(func(status *openrewrite_storage.JobStatus) bool {
		return status.JobID == "test-job-1" &&
			status.Status == openrewrite_storage.StatusQueued &&
			status.Progress == 0 &&
			status.Message == "Job queued for processing"
	})).Return(nil)
	
	err := q.Enqueue(job)
	assert.NoError(t, err)
	assert.Equal(t, 1, q.Size())
	
	mockStorage.AssertExpectations(t)
}

func TestJobQueue_EnqueueError(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	job := &Job{
		ID:        "test-job-2",
		Priority:  3,
		CreatedAt: time.Now(),
	}
	
	// Simulate storage error
	mockStorage.On("StoreJobStatus", "test-job-2", mock.Anything).Return(assert.AnError)
	
	err := q.Enqueue(job)
	assert.Error(t, err)
	assert.Equal(t, 0, q.Size(), "Job should not be added if status storage fails")
	
	mockStorage.AssertExpectations(t)
}

func TestJobQueue_Dequeue(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	// Add multiple jobs
	now := time.Now()
	job1 := &Job{
		ID:        "job-1",
		Priority:  1,
		CreatedAt: now,
	}
	job2 := &Job{
		ID:        "job-2",
		Priority:  5,
		CreatedAt: now,
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	
	require.NoError(t, q.Enqueue(job1))
	require.NoError(t, q.Enqueue(job2))
	
	// Dequeue should return highest priority job
	dequeued, err := q.Dequeue()
	assert.NoError(t, err)
	assert.NotNil(t, dequeued)
	assert.Equal(t, "job-2", dequeued.ID)
	assert.Equal(t, 1, q.Size())
	
	// Dequeue remaining job
	dequeued, err = q.Dequeue()
	assert.NoError(t, err)
	assert.NotNil(t, dequeued)
	assert.Equal(t, "job-1", dequeued.ID)
	assert.Equal(t, 0, q.Size())
	
	// Dequeue from empty queue
	dequeued, err = q.Dequeue()
	assert.Error(t, err)
	assert.Nil(t, dequeued)
	assert.Contains(t, err.Error(), "queue is empty")
}

func TestJobQueue_ConcurrentAccess(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	
	// Concurrently add jobs
	var wg sync.WaitGroup
	numJobs := 100
	
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			job := &Job{
				ID:        fmt.Sprintf("job-%d", id),
				Priority:  id % 5,
				CreatedAt: time.Now(),
			}
			err := q.Enqueue(job)
			assert.NoError(t, err)
		}(i)
	}
	
	wg.Wait()
	assert.Equal(t, numJobs, q.Size())
	
	// Concurrently dequeue jobs
	dequeued := make([]*Job, 0, numJobs)
	mu := sync.Mutex{}
	
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := q.Dequeue()
			if err == nil {
				mu.Lock()
				dequeued = append(dequeued, job)
				mu.Unlock()
			}
		}()
	}
	
	wg.Wait()
	assert.Equal(t, 0, q.Size())
	assert.Equal(t, numJobs, len(dequeued))
	
	// Verify all jobs were dequeued
	jobIDs := make(map[string]bool)
	for _, job := range dequeued {
		jobIDs[job.ID] = true
	}
	assert.Equal(t, numJobs, len(jobIDs), "All unique jobs should be dequeued")
}

func TestJobQueue_Peek(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	// Peek empty queue
	job, err := q.Peek()
	assert.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "queue is empty")
	
	// Add job and peek
	job1 := &Job{
		ID:        "job-1",
		Priority:  3,
		CreatedAt: time.Now(),
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, q.Enqueue(job1))
	
	peeked, err := q.Peek()
	assert.NoError(t, err)
	assert.NotNil(t, peeked)
	assert.Equal(t, "job-1", peeked.ID)
	assert.Equal(t, 1, q.Size(), "Peek should not remove job")
}

func TestJobQueue_UpdateMetrics(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	// Add some jobs
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	
	for i := 0; i < 3; i++ {
		job := &Job{
			ID:        fmt.Sprintf("job-%d", i),
			Priority:  i,
			CreatedAt: time.Now(),
		}
		require.NoError(t, q.Enqueue(job))
	}
	
	// Update metrics
	mockStorage.On("StoreMetrics", mock.MatchedBy(func(metrics *openrewrite_storage.Metrics) bool {
		return metrics.QueueDepth == 3
	})).Return(nil)
	
	err := q.UpdateMetrics()
	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestJobHeap_EdgeCases(t *testing.T) {
	h := &JobHeap{}
	heap.Init(h)
	
	// Test with same priority and timestamp
	now := time.Now()
	job1 := &Job{ID: "job-1", Priority: 5, CreatedAt: now}
	job2 := &Job{ID: "job-2", Priority: 5, CreatedAt: now}
	
	heap.Push(h, job1)
	heap.Push(h, job2)
	
	// Order is undefined but both should be retrievable
	first := heap.Pop(h).(*Job)
	second := heap.Pop(h).(*Job)
	
	assert.Contains(t, []string{"job-1", "job-2"}, first.ID)
	assert.Contains(t, []string{"job-1", "job-2"}, second.ID)
	assert.NotEqual(t, first.ID, second.ID)
}

func TestJobQueue_IsEmpty(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	assert.True(t, q.IsEmpty())
	
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, q.Enqueue(job))
	
	assert.False(t, q.IsEmpty())
}

func TestJobQueue_Clear(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, q.Enqueue(job))
	assert.Equal(t, 1, q.Size())
	
	q.Clear()
	assert.Equal(t, 0, q.Size())
	assert.True(t, q.IsEmpty())
}

func TestJobQueue_Contains(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	assert.False(t, q.Contains("nonexistent"))
	
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, q.Enqueue(job))
	
	assert.True(t, q.Contains("test-job-1"))
	assert.False(t, q.Contains("nonexistent"))
}

func TestJobQueue_RemoveJob(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	// Test removing non-existent job
	err := q.RemoveJob("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")
	
	// Add job and remove it
	job := &Job{
		ID:        "test-job-1",
		Priority:  3,
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, q.Enqueue(job))
	assert.True(t, q.Contains("test-job-1"))
	
	err = q.RemoveJob("test-job-1")
	assert.NoError(t, err)
	assert.False(t, q.Contains("test-job-1"))
}

func TestJobQueue_CancelJob(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	job := &Job{
		ID:        "test-job-cancel",
		Priority:  3,
		Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
		CreatedAt: time.Now(),
	}
	
	// Mock storing initial status and cancellation status
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	
	require.NoError(t, q.Enqueue(job))
	assert.True(t, q.Contains("test-job-cancel"))
	
	// Cancel the job
	err := q.CancelJob("test-job-cancel")
	assert.NoError(t, err)
	
	// Job should be removed from queue and marked as cancelled
	assert.False(t, q.Contains("test-job-cancel"))
	assert.True(t, q.IsCancelled("test-job-cancel"))
	
	mockStorage.AssertExpectations(t)
}

func TestJobQueue_PauseResume(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	assert.False(t, q.IsPaused())
	
	q.Pause()
	assert.True(t, q.IsPaused())
	
	q.Resume()
	assert.False(t, q.IsPaused())
}

func TestJobQueue_Lifecycle(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	assert.False(t, q.IsRunning())
	
	q.Start()
	assert.True(t, q.IsRunning())
	assert.Equal(t, 5, q.GetActiveWorkers())
	
	q.Stop()
	assert.False(t, q.IsRunning())
	assert.Equal(t, 0, q.GetActiveWorkers())
}

func TestJobQueue_Drain(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(5, mockStorage, new(MockExecutor))
	
	q.Start()
	assert.False(t, q.IsPaused())
	
	q.Drain()
	assert.True(t, q.IsPaused())
	
	q.Stop()
}

func TestJobQueue_EnhancedMetrics(t *testing.T) {
	mockStorage := new(MockJobStorage)
	q := NewJobQueue(3, mockStorage, new(MockExecutor))
	
	// Start queue to have active workers
	q.Start()
	defer q.Stop()
	
	// Add some jobs and cancel one
	mockStorage.On("StoreJobStatus", mock.Anything, mock.Anything).Return(nil)
	
	for i := 0; i < 2; i++ {
		job := &Job{
			ID:        fmt.Sprintf("test-job-%d", i),
			Priority:  i,
			Recipe:    openrewrite_storage.RecipeConfig{Name: "test-recipe"},
			CreatedAt: time.Now(),
		}
		require.NoError(t, q.Enqueue(job))
	}
	
	// Cancel one job
	require.NoError(t, q.CancelJob("test-job-0"))
	
	// Mock metrics storage with enhanced fields
	mockStorage.On("StoreMetrics", mock.MatchedBy(func(metrics *openrewrite_storage.Metrics) bool {
		return metrics.QueueDepth == 1 && // One job remaining (other was cancelled)
			metrics.ActiveWorkers == 3 && // 3 workers started
			metrics.JobsFailed == 1 // 1 cancelled job counted as failed
	})).Return(nil)
	
	err := q.UpdateMetrics()
	assert.NoError(t, err)
	
	mockStorage.AssertExpectations(t)
}