package openrewrite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockConsulStorage mocks Consul storage operations
type MockConsulStorage struct {
	mock.Mock
}

func (m *MockConsulStorage) StoreJobStatus(jobID string, status *JobStatus) error {
	args := m.Called(jobID, status)
	return args.Error(0)
}

func (m *MockConsulStorage) GetJobStatus(jobID string) (*JobStatus, error) {
	args := m.Called(jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*JobStatus), args.Error(1)
}

func (m *MockConsulStorage) WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error) {
	args := m.Called(jobID, index)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*JobStatus), args.Get(1).(uint64), args.Error(2)
}

func (m *MockConsulStorage) StoreMetrics(metrics *Metrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *MockConsulStorage) GetMetrics() (*Metrics, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Metrics), args.Error(1)
}

// MockSeaweedFSStorage mocks SeaweedFS storage operations
type MockSeaweedFSStorage struct {
	mock.Mock
}

func (m *MockSeaweedFSStorage) StoreDiff(jobID string, diff []byte) (string, error) {
	args := m.Called(jobID, diff)
	return args.String(0), args.Error(1)
}

func (m *MockSeaweedFSStorage) RetrieveDiff(fileID string) ([]byte, error) {
	args := m.Called(fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockSeaweedFSStorage) DeleteDiff(fileID string) error {
	args := m.Called(fileID)
	return args.Error(0)
}

func TestCompositeStorage_StoreJobStatus(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	now := time.Now()
	status := &JobStatus{
		JobID:     "test-job-1",
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		Progress:  0,
		Message:   "Job queued",
	}
	
	mockConsul.On("StoreJobStatus", "test-job-1", status).Return(nil)
	
	err := storage.StoreJobStatus("test-job-1", status)
	assert.NoError(t, err)
	mockConsul.AssertExpectations(t)
}

func TestCompositeStorage_GetJobStatus(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	expectedStatus := &JobStatus{
		JobID:   "test-job-2",
		Status:  StatusProcessing,
		Progress: 50,
		Message: "Processing",
	}
	
	mockConsul.On("GetJobStatus", "test-job-2").Return(expectedStatus, nil)
	
	status, err := storage.GetJobStatus("test-job-2")
	assert.NoError(t, err)
	assert.Equal(t, expectedStatus, status)
	mockConsul.AssertExpectations(t)
}

func TestCompositeStorage_GetJobStatusError(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	mockConsul.On("GetJobStatus", "nonexistent").Return(nil, errors.New("job not found"))
	
	status, err := storage.GetJobStatus("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "job not found")
	mockConsul.AssertExpectations(t)
}

func TestCompositeStorage_WatchJobStatus(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	expectedStatus := &JobStatus{
		JobID:   "test-job-3",
		Status:  StatusCompleted,
		Progress: 100,
	}
	
	mockConsul.On("WatchJobStatus", "test-job-3", uint64(10)).Return(expectedStatus, uint64(15), nil)
	
	status, index, err := storage.WatchJobStatus("test-job-3", 10)
	assert.NoError(t, err)
	assert.Equal(t, expectedStatus, status)
	assert.Equal(t, uint64(15), index)
	mockConsul.AssertExpectations(t)
}

func TestCompositeStorage_StoreDiff(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	diff := []byte("diff content here")
	expectedFileID := "3,01234567890123"
	
	mockSeaweed.On("StoreDiff", "test-job-4", diff).Return(expectedFileID, nil)
	
	fileID, err := storage.StoreDiff("test-job-4", diff)
	assert.NoError(t, err)
	assert.Equal(t, expectedFileID, fileID)
	mockSeaweed.AssertExpectations(t)
}

func TestCompositeStorage_StoreDiffError(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	diff := []byte("diff content")
	mockSeaweed.On("StoreDiff", "test-job-5", diff).Return("", errors.New("storage error"))
	
	fileID, err := storage.StoreDiff("test-job-5", diff)
	assert.Error(t, err)
	assert.Empty(t, fileID)
	assert.Contains(t, err.Error(), "storage error")
	mockSeaweed.AssertExpectations(t)
}

func TestCompositeStorage_RetrieveDiff(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	expectedDiff := []byte("retrieved diff content")
	fileID := "3,01234567890123"
	
	mockSeaweed.On("RetrieveDiff", fileID).Return(expectedDiff, nil)
	
	diff, err := storage.RetrieveDiff(fileID)
	assert.NoError(t, err)
	assert.Equal(t, expectedDiff, diff)
	mockSeaweed.AssertExpectations(t)
}

func TestCompositeStorage_DeleteDiff(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	fileID := "5,abcdef123456"
	mockSeaweed.On("DeleteDiff", fileID).Return(nil)
	
	err := storage.DeleteDiff(fileID)
	assert.NoError(t, err)
	mockSeaweed.AssertExpectations(t)
}

func TestCompositeStorage_StoreMetrics(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	metrics := &Metrics{
		QueueDepth:    10,
		ActiveWorkers: 5,
		LastActivity:  time.Now(),
		JobsProcessed: 100,
		JobsFailed:    2,
	}
	
	mockConsul.On("StoreMetrics", metrics).Return(nil)
	
	err := storage.StoreMetrics(metrics)
	assert.NoError(t, err)
	mockConsul.AssertExpectations(t)
}

func TestCompositeStorage_GetMetrics(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	expectedMetrics := &Metrics{
		QueueDepth:    5,
		ActiveWorkers: 3,
		JobsProcessed: 50,
		JobsFailed:    1,
	}
	
	mockConsul.On("GetMetrics").Return(expectedMetrics, nil)
	
	metrics, err := storage.GetMetrics()
	assert.NoError(t, err)
	assert.Equal(t, expectedMetrics, metrics)
	mockConsul.AssertExpectations(t)
}

func TestNewCompositeStorage(t *testing.T) {
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := NewCompositeStorage(mockConsul, mockSeaweed)
	assert.NotNil(t, storage)
	assert.Equal(t, mockConsul, storage.consul)
	assert.Equal(t, mockSeaweed, storage.seaweed)
}

func TestCompositeStorage_CompleteJobWorkflow(t *testing.T) {
	// Test a complete job workflow: create, update, store diff, complete
	mockConsul := new(MockConsulStorage)
	mockSeaweed := new(MockSeaweedFSStorage)
	
	storage := &CompositeStorage{
		consul:   mockConsul,
		seaweed:  mockSeaweed,
	}
	
	jobID := "workflow-test"
	now := time.Now()
	
	// 1. Store initial status (queued)
	queuedStatus := &JobStatus{
		JobID:     jobID,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		Progress:  0,
		Message:   "Job queued",
	}
	mockConsul.On("StoreJobStatus", jobID, queuedStatus).Return(nil)
	
	err := storage.StoreJobStatus(jobID, queuedStatus)
	assert.NoError(t, err)
	
	// 2. Update to processing
	processingStatus := &JobStatus{
		JobID:     jobID,
		Status:    StatusProcessing,
		CreatedAt: now,
		UpdatedAt: now.Add(1 * time.Second),
		Progress:  50,
		Message:   "Processing transformation",
	}
	mockConsul.On("StoreJobStatus", jobID, processingStatus).Return(nil)
	
	err = storage.StoreJobStatus(jobID, processingStatus)
	assert.NoError(t, err)
	
	// 3. Store diff
	diff := []byte("transformation diff")
	fileID := "7,workflow123456"
	mockSeaweed.On("StoreDiff", jobID, diff).Return(fileID, nil)
	
	storedFileID, err := storage.StoreDiff(jobID, diff)
	assert.NoError(t, err)
	assert.Equal(t, fileID, storedFileID)
	
	// 4. Update to completed with diff URL
	completedTime := now.Add(2 * time.Second)
	completedStatus := &JobStatus{
		JobID:       jobID,
		Status:      StatusCompleted,
		CreatedAt:   now,
		UpdatedAt:   completedTime,
		CompletedAt: &completedTime,
		Progress:    100,
		Message:     "Transformation completed",
		DiffURL:     fileID,
	}
	mockConsul.On("StoreJobStatus", jobID, completedStatus).Return(nil)
	
	err = storage.StoreJobStatus(jobID, completedStatus)
	assert.NoError(t, err)
	
	// 5. Retrieve completed status
	mockConsul.On("GetJobStatus", jobID).Return(completedStatus, nil)
	
	finalStatus, err := storage.GetJobStatus(jobID)
	assert.NoError(t, err)
	assert.Equal(t, StatusCompleted, finalStatus.Status)
	assert.Equal(t, fileID, finalStatus.DiffURL)
	
	// 6. Retrieve diff
	mockSeaweed.On("RetrieveDiff", fileID).Return(diff, nil)
	
	retrievedDiff, err := storage.RetrieveDiff(fileID)
	assert.NoError(t, err)
	assert.Equal(t, diff, retrievedDiff)
	
	mockConsul.AssertExpectations(t)
	mockSeaweed.AssertExpectations(t)
}