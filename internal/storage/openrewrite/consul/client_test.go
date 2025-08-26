package consul

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/internal/storage/openrewrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockConsulKV mocks the Consul KV interface
type MockConsulKV struct {
	mock.Mock
}

func (m *MockConsulKV) Put(p *api.KVPair, q *api.WriteOptions) (*api.WriteMeta, error) {
	args := m.Called(p, q)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.WriteMeta), args.Error(1)
}

func (m *MockConsulKV) Get(key string, q *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error) {
	args := m.Called(key, q)
	if args.Get(0) == nil {
		return nil, args.Get(1).(*api.QueryMeta), args.Error(2)
	}
	return args.Get(0).(*api.KVPair), args.Get(1).(*api.QueryMeta), args.Error(2)
}

func TestNewConsulStorage(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "valid address",
			address: "127.0.0.1:8500",
			wantErr: false,
		},
		{
			name:    "empty address uses default",
			address: "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewConsulStorage(tt.address)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, storage)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, storage)
				assert.NotNil(t, storage.client)
				assert.Equal(t, "ploy/openrewrite/jobs", storage.prefix)
			}
		})
	}
}

func TestStoreJobStatus(t *testing.T) {
	mockKV := new(MockConsulKV)
	storage := &ConsulStorage{
		kv:     mockKV,
		prefix: "ploy/openrewrite/jobs",
	}

	jobID := "job-123"
	status := &openrewrite.JobStatus{
		JobID:     jobID,
		Status:    "queued",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Message:   "Job queued for processing",
		Progress:  0,
	}

	expectedKey := "ploy/openrewrite/jobs/job-123"
	
	mockKV.On("Put", mock.MatchedBy(func(p *api.KVPair) bool {
		// Verify the key
		if p.Key != expectedKey {
			return false
		}
		
		// Verify the value can be unmarshaled back to JobStatus
		var stored openrewrite.JobStatus
		err := json.Unmarshal(p.Value, &stored)
		if err != nil {
			return false
		}
		
		return stored.JobID == jobID && stored.Status == "queued"
	}), (*api.WriteOptions)(nil)).Return(&api.WriteMeta{}, nil)

	err := storage.StoreJobStatus(jobID, status)
	assert.NoError(t, err)
	mockKV.AssertExpectations(t)
}

func TestGetJobStatus(t *testing.T) {
	mockKV := new(MockConsulKV)
	storage := &ConsulStorage{
		kv:     mockKV,
		prefix: "ploy/openrewrite/jobs",
	}

	t.Run("job exists", func(t *testing.T) {
		jobID := "job-123"
		expectedKey := "ploy/openrewrite/jobs/job-123"
		
		status := &openrewrite.JobStatus{
			JobID:     jobID,
			Status:    "processing",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Message:   "Processing transformation",
			Progress:  50,
		}
		
		statusJSON, _ := json.Marshal(status)
		kvPair := &api.KVPair{
			Key:   expectedKey,
			Value: statusJSON,
		}
		
		mockKV.On("Get", expectedKey, (*api.QueryOptions)(nil)).Return(kvPair, &api.QueryMeta{}, nil)
		
		result, err := storage.GetJobStatus(jobID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, jobID, result.JobID)
		assert.Equal(t, "processing", result.Status)
		assert.Equal(t, 50, result.Progress)
		
		mockKV.AssertExpectations(t)
	})

	t.Run("job not found", func(t *testing.T) {
		jobID := "job-456"
		expectedKey := "ploy/openrewrite/jobs/job-456"
		
		mockKV.On("Get", expectedKey, (*api.QueryOptions)(nil)).Return((*api.KVPair)(nil), &api.QueryMeta{}, nil)
		
		result, err := storage.GetJobStatus(jobID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "job not found")
		
		mockKV.AssertExpectations(t)
	})
}

func TestWatchJobStatus(t *testing.T) {
	mockKV := new(MockConsulKV)
	storage := &ConsulStorage{
		kv:     mockKV,
		prefix: "ploy/openrewrite/jobs",
	}

	jobID := "job-789"
	expectedKey := "ploy/openrewrite/jobs/job-789"
	waitIndex := uint64(100)
	
	status := &openrewrite.JobStatus{
		JobID:     jobID,
		Status:    "completed",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Message:   "Transformation completed successfully",
		Progress:  100,
	}
	
	statusJSON, _ := json.Marshal(status)
	kvPair := &api.KVPair{
		Key:   expectedKey,
		Value: statusJSON,
	}
	
	queryMeta := &api.QueryMeta{
		LastIndex: 101,
	}
	
	// Match the query options with the expected wait index
	mockKV.On("Get", expectedKey, mock.MatchedBy(func(q *api.QueryOptions) bool {
		return q.WaitIndex == waitIndex && q.WaitTime == 30*time.Second
	})).Return(kvPair, queryMeta, nil)
	
	result, newIndex, err := storage.WatchJobStatus(jobID, waitIndex)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, uint64(101), newIndex)
	
	mockKV.AssertExpectations(t)
}

func TestJobStatusLifecycle(t *testing.T) {
	// Test the complete lifecycle of a job status
	mockKV := new(MockConsulKV)
	storage := &ConsulStorage{
		kv:     mockKV,
		prefix: "ploy/openrewrite/jobs",
	}

	jobID := "job-lifecycle"
	
	// 1. Store initial status (queued)
	queuedStatus := &openrewrite.JobStatus{
		JobID:     jobID,
		Status:    "queued",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Message:   "Job queued for processing",
		Progress:  0,
	}
	
	mockKV.On("Put", mock.Anything, (*api.WriteOptions)(nil)).Return(&api.WriteMeta{}, nil).Once()
	err := storage.StoreJobStatus(jobID, queuedStatus)
	assert.NoError(t, err)
	
	// 2. Update to processing
	processingStatus := &openrewrite.JobStatus{
		JobID:     jobID,
		Status:    "processing",
		CreatedAt: queuedStatus.CreatedAt,
		UpdatedAt: time.Now(),
		Message:   "Starting transformation",
		Progress:  10,
	}
	
	mockKV.On("Put", mock.Anything, (*api.WriteOptions)(nil)).Return(&api.WriteMeta{}, nil).Once()
	err = storage.StoreJobStatus(jobID, processingStatus)
	assert.NoError(t, err)
	
	// 3. Update to completed
	completedAt := time.Now()
	completedStatus := &openrewrite.JobStatus{
		JobID:       jobID,
		Status:      "completed",
		CreatedAt:   queuedStatus.CreatedAt,
		UpdatedAt:   time.Now(),
		CompletedAt: &completedAt,
		Message:     "Transformation completed successfully",
		Progress:    100,
		DiffURL:     "seaweedfs://diff-123",
	}
	
	mockKV.On("Put", mock.Anything, (*api.WriteOptions)(nil)).Return(&api.WriteMeta{}, nil).Once()
	err = storage.StoreJobStatus(jobID, completedStatus)
	assert.NoError(t, err)
	
	mockKV.AssertExpectations(t)
}

func TestStoreAndGetMetrics(t *testing.T) {
	t.Run("store metrics", func(t *testing.T) {
		mockKV := new(MockConsulKV)
		storage := &ConsulStorage{
			kv:     mockKV,
			prefix: "ploy/openrewrite/jobs",
		}
		metrics := &openrewrite.Metrics{
			QueueDepth:    5,
			ActiveWorkers: 2,
			LastActivity:  time.Now(),
			JobsProcessed: 100,
			JobsFailed:    3,
		}

		expectedKey := "ploy/openrewrite/jobs/../metrics"
		
		mockKV.On("Put", mock.MatchedBy(func(p *api.KVPair) bool {
			// Verify the key
			if p.Key != expectedKey {
				return false
			}
			
			// Verify the value can be unmarshaled back to Metrics
			var stored openrewrite.Metrics
			err := json.Unmarshal(p.Value, &stored)
			if err != nil {
				return false
			}
			
			return stored.QueueDepth == 5 && stored.ActiveWorkers == 2
		}), (*api.WriteOptions)(nil)).Return(&api.WriteMeta{}, nil)

		err := storage.StoreMetrics(metrics)
		assert.NoError(t, err)
		mockKV.AssertExpectations(t)
	})

	t.Run("get metrics exists", func(t *testing.T) {
		mockKV := new(MockConsulKV)
		storage := &ConsulStorage{
			kv:     mockKV,
			prefix: "ploy/openrewrite/jobs",
		}
		
		expectedKey := "ploy/openrewrite/jobs/../metrics"
		
		metrics := &openrewrite.Metrics{
			QueueDepth:    10,
			ActiveWorkers: 4,
			LastActivity:  time.Now(),
			JobsProcessed: 200,
			JobsFailed:    5,
		}
		
		metricsJSON, _ := json.Marshal(metrics)
		kvPair := &api.KVPair{
			Key:   expectedKey,
			Value: metricsJSON,
		}
		
		mockKV.On("Get", expectedKey, (*api.QueryOptions)(nil)).Return(kvPair, &api.QueryMeta{}, nil)
		
		result, err := storage.GetMetrics()
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 10, result.QueueDepth)
		assert.Equal(t, 4, result.ActiveWorkers)
		assert.Equal(t, int64(200), result.JobsProcessed)
		
		mockKV.AssertExpectations(t)
	})

	t.Run("get metrics not found returns default", func(t *testing.T) {
		mockKV := new(MockConsulKV)
		storage := &ConsulStorage{
			kv:     mockKV,
			prefix: "ploy/openrewrite/jobs",
		}
		
		expectedKey := "ploy/openrewrite/jobs/../metrics"
		
		mockKV.On("Get", expectedKey, (*api.QueryOptions)(nil)).Return((*api.KVPair)(nil), &api.QueryMeta{}, nil)
		
		result, err := storage.GetMetrics()
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.QueueDepth)
		assert.Equal(t, 0, result.ActiveWorkers)
		// LastActivity should be set to a recent time
		assert.WithinDuration(t, time.Now(), result.LastActivity, 1*time.Second)
		
		mockKV.AssertExpectations(t)
	})
}