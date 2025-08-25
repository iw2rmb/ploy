package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stretchr/testify/mock"
)

// JobStatus represents the status of a Nomad job
type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusDead    JobStatus = "dead"
	JobStatusFailed  JobStatus = "failed"
)

// MockJob represents a mock Nomad job
type MockJob struct {
	ID          string
	Name        string
	Status      JobStatus
	Allocations []MockAllocation
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MockAllocation represents a mock Nomad allocation
type MockAllocation struct {
	ID           string
	JobID        string
	NodeID       string
	TaskGroup    string
	Status       string
	DesiredStatus string
	ClientStatus string
	CreatedAt    time.Time
}

// MockNomadClient is a mock implementation of the Nomad client
type MockNomadClient struct {
	mock.Mock
	mu   sync.RWMutex
	jobs map[string]*MockJob
}

// NewMockNomadClient creates a new mock Nomad client
func NewMockNomadClient() *MockNomadClient {
	return &MockNomadClient{
		jobs: make(map[string]*MockJob),
	}
}

// SubmitJob submits a new job to Nomad
func (m *MockNomadClient) SubmitJob(ctx context.Context, jobSpec string) (*MockJob, error) {
	args := m.Called(ctx, jobSpec)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	// Create a mock job
	job := &MockJob{
		ID:        fmt.Sprintf("job-%d", len(m.jobs)+1),
		Name:      fmt.Sprintf("test-job-%d", len(m.jobs)+1),
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the job
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = job

	return job, nil
}

// GetJob retrieves a job by ID
func (m *MockNomadClient) GetJob(ctx context.Context, jobID string) (*MockJob, error) {
	args := m.Called(ctx, jobID)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	job, exists := m.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return job, nil
}

// StopJob stops a running job
func (m *MockNomadClient) StopJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	job, exists := m.jobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.Status = JobStatusDead
	job.UpdatedAt = time.Now()

	return nil
}

// ListJobs lists all jobs
func (m *MockNomadClient) ListJobs(ctx context.Context) ([]*MockJob, error) {
	args := m.Called(ctx)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make([]*MockJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetJobAllocations retrieves allocations for a job
func (m *MockNomadClient) GetJobAllocations(ctx context.Context, jobID string) ([]MockAllocation, error) {
	args := m.Called(ctx, jobID)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	job, exists := m.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return job.Allocations, nil
}

// ScaleJob scales a job to the specified count
func (m *MockNomadClient) ScaleJob(ctx context.Context, jobID string, count int) error {
	args := m.Called(ctx, jobID, count)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	job, exists := m.jobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	// Simulate scaling by adjusting allocations
	job.Allocations = make([]MockAllocation, count)
	for i := 0; i < count; i++ {
		job.Allocations[i] = MockAllocation{
			ID:            fmt.Sprintf("%s-alloc-%d", jobID, i),
			JobID:         jobID,
			NodeID:        fmt.Sprintf("node-%d", i%3),
			TaskGroup:     "app",
			Status:        "running",
			DesiredStatus: "run",
			ClientStatus:  "running",
			CreatedAt:     time.Now(),
		}
	}

	job.UpdatedAt = time.Now()
	return nil
}

// WaitForJobStatus waits for a job to reach the specified status
func (m *MockNomadClient) WaitForJobStatus(ctx context.Context, jobID string, status JobStatus, timeout time.Duration) error {
	args := m.Called(ctx, jobID, status, timeout)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	// Simulate waiting by checking job status
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		job, exists := m.jobs[jobID]
		m.mu.RUnlock()

		if !exists {
			return fmt.Errorf("job not found: %s", jobID)
		}

		if job.Status == status {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return fmt.Errorf("timeout waiting for job %s to reach status %s", jobID, status)
}

// GetNodeInfo retrieves information about cluster nodes
func (m *MockNomadClient) GetNodeInfo(ctx context.Context) ([]MockNode, error) {
	args := m.Called(ctx)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	// Return mock node information
	nodes := []MockNode{
		{
			ID:       "node-1",
			Name:     "nomad-client-1",
			Status:   "ready",
			Address:  "172.20.0.100:4646",
			Datacenter: "local",
		},
		{
			ID:       "node-2",
			Name:     "nomad-client-2", 
			Status:   "ready",
			Address:  "172.20.0.101:4646",
			Datacenter: "local",
		},
	}

	return nodes, nil
}

// Health checks the health of the Nomad cluster
func (m *MockNomadClient) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Close closes the Nomad client
func (m *MockNomadClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockNode represents a Nomad node
type MockNode struct {
	ID         string
	Name       string
	Status     string
	Address    string
	Datacenter string
}

// SetupDefault sets up default mock behavior
func (m *MockNomadClient) SetupDefault() {
	m.On("SubmitJob", mock.Anything, mock.Anything).Return(&MockJob{
		ID:     "test-job-1",
		Name:   "test-job",
		Status: JobStatusRunning,
	}, nil)
	
	m.On("GetJob", mock.Anything, mock.Anything).Return(&MockJob{
		ID:     "test-job-1",
		Name:   "test-job",
		Status: JobStatusRunning,
	}, nil)
	
	m.On("StopJob", mock.Anything, mock.Anything).Return(nil)
	m.On("ListJobs", mock.Anything).Return([]*MockJob{}, nil)
	m.On("GetJobAllocations", mock.Anything, mock.Anything).Return([]MockAllocation{}, nil)
	m.On("ScaleJob", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("WaitForJobStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("GetNodeInfo", mock.Anything).Return([]MockNode{}, nil)
	m.On("Health", mock.Anything).Return(nil)
	m.On("Close").Return(nil)
}

// SimulateJobTransition simulates a job status transition
func (m *MockNomadClient) SimulateJobTransition(jobID string, newStatus JobStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if job, exists := m.jobs[jobID]; exists {
		job.Status = newStatus
		job.UpdatedAt = time.Now()
	}
}

// SimulateFailure configures the mock to simulate Nomad failures
func (m *MockNomadClient) SimulateFailure(operation string, err error) {
	switch operation {
	case "submit":
		m.On("SubmitJob", mock.Anything, mock.Anything).Return(nil, err)
	case "get":
		m.On("GetJob", mock.Anything, mock.Anything).Return(nil, err)
	case "stop":
		m.On("StopJob", mock.Anything, mock.Anything).Return(err)
	case "health":
		m.On("Health", mock.Anything).Return(err)
	}
}

// GetStoredJobs returns all stored jobs (for testing purposes)
func (m *MockNomadClient) GetStoredJobs() map[string]*MockJob {
	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make(map[string]*MockJob)
	for k, v := range m.jobs {
		jobs[k] = v
	}
	return jobs
}

// ClearJobs clears all stored jobs
func (m *MockNomadClient) ClearJobs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.jobs = make(map[string]*MockJob)
}