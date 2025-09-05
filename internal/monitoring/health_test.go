package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorageProvider mocks the storage provider for health checks
type MockStorageProvider struct {
	mock.Mock
}

func (m *MockStorageProvider) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockConsulClient mocks the Consul client for health checks
type MockConsulClient struct {
	mock.Mock
}

func (m *MockConsulClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockWorkerPool mocks the worker pool for utilization checks
type MockWorkerPool struct {
	mock.Mock
}

func (m *MockWorkerPool) GetUtilization() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

func (m *MockWorkerPool) GetActiveWorkers() int {
	args := m.Called()
	return args.Get(0).(int)
}

func (m *MockWorkerPool) GetTotalWorkers() int {
	args := m.Called()
	return args.Get(0).(int)
}

func TestNewHealthChecker(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	assert.NotNil(t, checker)
	assert.NotNil(t, checker.checks)
	assert.Equal(t, 3, len(checker.checks))
}

func TestHealthChecker_GetHealth_AllHealthy(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock healthy responses
	storage.On("Ping", mock.Anything).Return(nil)
	consul.On("Ping", mock.Anything).Return(nil)
	worker.On("GetUtilization").Return(0.5) // 50% utilization

	checker := NewHealthChecker(storage, consul, worker)
	ctx := context.Background()

	status := checker.GetHealth(ctx)

	assert.Equal(t, StatusHealthy, status.Status)
	assert.Equal(t, 3, len(status.Checks))

	// Verify all checks are healthy
	for name, check := range status.Checks {
		assert.True(t, check.Healthy, "Check %s should be healthy", name)
		assert.Empty(t, check.Error)
	}

	storage.AssertExpectations(t)
	consul.AssertExpectations(t)
	worker.AssertExpectations(t)
}

func TestHealthChecker_GetHealth_StorageUnhealthy(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock storage failure
	storage.On("Ping", mock.Anything).Return(errors.New("storage connection failed"))
	consul.On("Ping", mock.Anything).Return(nil)
	worker.On("GetUtilization").Return(0.5)

	checker := NewHealthChecker(storage, consul, worker)
	ctx := context.Background()

	status := checker.GetHealth(ctx)

	assert.Equal(t, StatusUnhealthy, status.Status)
	assert.False(t, status.Checks["seaweedfs"].Healthy)
	assert.Equal(t, "storage connection failed", status.Checks["seaweedfs"].Error)
	assert.True(t, status.Checks["consul"].Healthy)
	assert.True(t, status.Checks["worker_pool"].Healthy)

	storage.AssertExpectations(t)
	consul.AssertExpectations(t)
	worker.AssertExpectations(t)
}

func TestHealthChecker_GetHealth_ConsulUnhealthy(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock consul failure
	storage.On("Ping", mock.Anything).Return(nil)
	consul.On("Ping", mock.Anything).Return(errors.New("consul connection failed"))
	worker.On("GetUtilization").Return(0.5)

	checker := NewHealthChecker(storage, consul, worker)
	ctx := context.Background()

	status := checker.GetHealth(ctx)

	assert.Equal(t, StatusDegraded, status.Status) // Degraded because storage is still healthy
	assert.True(t, status.Checks["seaweedfs"].Healthy)
	assert.False(t, status.Checks["consul"].Healthy)
	assert.Equal(t, "consul connection failed", status.Checks["consul"].Error)
	assert.True(t, status.Checks["worker_pool"].Healthy)

	storage.AssertExpectations(t)
	consul.AssertExpectations(t)
	worker.AssertExpectations(t)
}

func TestHealthChecker_GetHealth_WorkerPoolOverloaded(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock high utilization
	storage.On("Ping", mock.Anything).Return(nil)
	consul.On("Ping", mock.Anything).Return(nil)
	worker.On("GetUtilization").Return(0.95) // 95% utilization

	checker := NewHealthChecker(storage, consul, worker)
	ctx := context.Background()

	status := checker.GetHealth(ctx)

	assert.Equal(t, StatusDegraded, status.Status)
	assert.True(t, status.Checks["seaweedfs"].Healthy)
	assert.True(t, status.Checks["consul"].Healthy)
	assert.False(t, status.Checks["worker_pool"].Healthy)
	assert.Equal(t, "worker pool overloaded: 95.00% utilization", status.Checks["worker_pool"].Error)

	storage.AssertExpectations(t)
	consul.AssertExpectations(t)
	worker.AssertExpectations(t)
}

func TestHealthChecker_GetHealth_MultipleFailures(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock multiple failures
	storage.On("Ping", mock.Anything).Return(errors.New("storage down"))
	consul.On("Ping", mock.Anything).Return(errors.New("consul down"))
	worker.On("GetUtilization").Return(0.98) // 98% utilization

	checker := NewHealthChecker(storage, consul, worker)
	ctx := context.Background()

	status := checker.GetHealth(ctx)

	assert.Equal(t, StatusUnhealthy, status.Status)
	assert.False(t, status.Checks["seaweedfs"].Healthy)
	assert.False(t, status.Checks["consul"].Healthy)
	assert.False(t, status.Checks["worker_pool"].Healthy)

	storage.AssertExpectations(t)
	consul.AssertExpectations(t)
	worker.AssertExpectations(t)
}

func TestHealthChecker_GetHealth_WithTimeout(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	// Mock slow storage that will timeout
	storage.On("Ping", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		time.Sleep(100 * time.Millisecond)
	})
	consul.On("Ping", mock.Anything).Return(nil)
	worker.On("GetUtilization").Return(0.5)

	checker := NewHealthChecker(storage, consul, worker)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	status := checker.GetHealth(ctx)

	// Should handle timeout gracefully
	assert.NotNil(t, status)
	// At least consul and worker should have been checked
	assert.NotNil(t, status.Checks)
}

func TestHealthChecker_CheckStorage(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	t.Run("healthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(nil).Once()

		check := checker.checkStorage(context.Background())

		assert.True(t, check.Healthy)
		assert.Empty(t, check.Error)
		assert.WithinDuration(t, time.Now(), check.Timestamp, 1*time.Second)
		storage.AssertExpectations(t)
	})

	t.Run("unhealthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(errors.New("connection refused")).Once()

		check := checker.checkStorage(context.Background())

		assert.False(t, check.Healthy)
		assert.Equal(t, "connection refused", check.Error)
		assert.WithinDuration(t, time.Now(), check.Timestamp, 1*time.Second)
		storage.AssertExpectations(t)
	})
}

func TestHealthChecker_CheckConsul(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	t.Run("healthy", func(t *testing.T) {
		consul.On("Ping", mock.Anything).Return(nil).Once()

		check := checker.checkConsul(context.Background())

		assert.True(t, check.Healthy)
		assert.Empty(t, check.Error)
		assert.WithinDuration(t, time.Now(), check.Timestamp, 1*time.Second)
		consul.AssertExpectations(t)
	})

	t.Run("unhealthy", func(t *testing.T) {
		consul.On("Ping", mock.Anything).Return(errors.New("consul unavailable")).Once()

		check := checker.checkConsul(context.Background())

		assert.False(t, check.Healthy)
		assert.Equal(t, "consul unavailable", check.Error)
		assert.WithinDuration(t, time.Now(), check.Timestamp, 1*time.Second)
		consul.AssertExpectations(t)
	})
}

func TestHealthChecker_CheckWorkerPool(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	t.Run("healthy_low_utilization", func(t *testing.T) {
		worker.On("GetUtilization").Return(0.3).Once() // 30% utilization

		check := checker.checkWorkerPool(context.Background())

		assert.True(t, check.Healthy)
		assert.Empty(t, check.Error)
		assert.WithinDuration(t, time.Now(), check.Timestamp, 1*time.Second)
		worker.AssertExpectations(t)
	})

	t.Run("healthy_moderate_utilization", func(t *testing.T) {
		worker.On("GetUtilization").Return(0.75).Once() // 75% utilization

		check := checker.checkWorkerPool(context.Background())

		assert.True(t, check.Healthy)
		assert.Empty(t, check.Error)
		worker.AssertExpectations(t)
	})

	t.Run("unhealthy_high_utilization", func(t *testing.T) {
		worker.On("GetUtilization").Return(0.91).Once() // 91% utilization

		check := checker.checkWorkerPool(context.Background())

		assert.False(t, check.Healthy)
		assert.Equal(t, "worker pool overloaded: 91.00% utilization", check.Error)
		worker.AssertExpectations(t)
	})

	t.Run("unhealthy_full_utilization", func(t *testing.T) {
		worker.On("GetUtilization").Return(1.0).Once() // 100% utilization

		check := checker.checkWorkerPool(context.Background())

		assert.False(t, check.Healthy)
		assert.Equal(t, "worker pool overloaded: 100.00% utilization", check.Error)
		worker.AssertExpectations(t)
	})
}

func TestHealthChecker_GetReadiness(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	t.Run("ready_when_healthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(nil).Once()
		consul.On("Ping", mock.Anything).Return(nil).Once()
		worker.On("GetUtilization").Return(0.5).Once()

		ready := checker.GetReadiness(context.Background())
		assert.True(t, ready)

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})

	t.Run("ready_when_degraded", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(nil).Once()
		consul.On("Ping", mock.Anything).Return(errors.New("consul error")).Once()
		worker.On("GetUtilization").Return(0.5).Once()

		ready := checker.GetReadiness(context.Background())
		assert.True(t, ready) // Still ready when degraded

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})

	t.Run("not_ready_when_unhealthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(errors.New("storage down")).Once()
		consul.On("Ping", mock.Anything).Return(nil).Once()
		worker.On("GetUtilization").Return(0.5).Once()

		ready := checker.GetReadiness(context.Background())
		assert.False(t, ready) // Not ready when unhealthy

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})
}

func TestHealthChecker_GetLiveness(t *testing.T) {
	storage := &MockStorageProvider{}
	consul := &MockConsulClient{}
	worker := &MockWorkerPool{}

	checker := NewHealthChecker(storage, consul, worker)

	t.Run("alive_when_healthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(nil).Once()
		consul.On("Ping", mock.Anything).Return(nil).Once()
		worker.On("GetUtilization").Return(0.5).Once()

		alive := checker.GetLiveness(context.Background())
		assert.True(t, alive)

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})

	t.Run("alive_when_degraded", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(nil).Once()
		consul.On("Ping", mock.Anything).Return(errors.New("consul error")).Once()
		worker.On("GetUtilization").Return(0.5).Once()

		alive := checker.GetLiveness(context.Background())
		assert.True(t, alive) // Still alive when degraded

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})

	t.Run("not_alive_when_unhealthy", func(t *testing.T) {
		storage.On("Ping", mock.Anything).Return(errors.New("storage down")).Once()
		consul.On("Ping", mock.Anything).Return(errors.New("consul down")).Once()
		worker.On("GetUtilization").Return(0.98).Once()

		alive := checker.GetLiveness(context.Background())
		assert.False(t, alive) // Not alive when everything is failing

		storage.AssertExpectations(t)
		consul.AssertExpectations(t)
		worker.AssertExpectations(t)
	})
}
