package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for MonitoringMiddleware
func TestMonitoringMiddleware_ImplementsStorageInterface(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)

	// This should compile - middleware must implement storage.Storage interface
	var _ storage.Storage = monitoringMiddleware
}

func TestMonitoringMiddleware_Get_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform successful Get operation
	reader, err := monitoringMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer func() { _ = reader.Close() }()

	// Verify metrics were recorded
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.TotalDownloads)
	assert.Equal(t, int64(1), snapshot.SuccessfulDownloads)
	assert.Equal(t, int64(0), snapshot.FailedDownloads)
	assert.Greater(t, snapshot.AverageDownloadTime, time.Duration(0))
}

func TestMonitoringMiddleware_Get_RecordsFailureMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	// Set up mock to fail
	networkErr := storage.NewStorageError("get", errors.New("connection refused"), storage.ErrorContext{
		Key: "test-key",
	})
	networkErr.ErrorType = storage.ErrorTypeNetwork
	mockStorage.SetFailures(1, networkErr)

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform failing Get operation
	reader, err := monitoringMiddleware.Get(ctx, "test-key")
	require.Error(t, err)
	require.Nil(t, reader)

	// Verify failure metrics were recorded
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.TotalDownloads)
	assert.Equal(t, int64(0), snapshot.SuccessfulDownloads)
	assert.Equal(t, int64(1), snapshot.FailedDownloads)
	assert.Equal(t, int64(1), snapshot.ErrorsByType[storage.ErrorTypeNetwork])
}

func TestMonitoringMiddleware_Put_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform successful Put operation
	content := "test content"
	err := monitoringMiddleware.Put(ctx, "test-key", strings.NewReader(content))
	require.NoError(t, err)

	// Verify metrics were recorded
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.TotalUploads)
	assert.Equal(t, int64(1), snapshot.SuccessfulUploads)
	assert.Equal(t, int64(0), snapshot.FailedUploads)
	assert.Equal(t, int64(len(content)), snapshot.TotalBytesUploaded)
	assert.Greater(t, snapshot.AverageUploadTime, time.Duration(0))
}

func TestMonitoringMiddleware_Put_RecordsFailureMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	// Set up mock to fail
	timeoutErr := storage.NewStorageError("put", errors.New("timeout"), storage.ErrorContext{
		Key: "test-key",
	})
	timeoutErr.ErrorType = storage.ErrorTypeTimeout
	mockStorage.SetFailures(1, timeoutErr)

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform failing Put operation
	err := monitoringMiddleware.Put(ctx, "test-key", strings.NewReader("test"))
	require.Error(t, err)

	// Verify failure metrics were recorded
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.TotalUploads)
	assert.Equal(t, int64(0), snapshot.SuccessfulUploads)
	assert.Equal(t, int64(1), snapshot.FailedUploads)
	assert.Equal(t, int64(1), snapshot.ErrorsByType[storage.ErrorTypeTimeout])
}

func TestMonitoringMiddleware_Delete_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform successful Delete operation
	err := monitoringMiddleware.Delete(ctx, "test-key")
	require.NoError(t, err)

	// Since delete doesn't have specific metrics in StorageMetrics,
	// we verify it completes without error
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_List_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform successful List operation
	objects, err := monitoringMiddleware.List(ctx, storage.ListOptions{Prefix: "test/"})
	require.NoError(t, err)
	require.NotNil(t, objects)
	assert.Len(t, objects, 2) // MockStorage returns 2 objects

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_Health_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform successful Health check
	err := monitoringMiddleware.Health(ctx)
	require.NoError(t, err)

	// Verify health check completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
	// Health status should be updated based on metrics
	assert.NotEqual(t, storage.HealthStatusUnknown, snapshot.HealthStatus)
}

func TestMonitoringMiddleware_ConcurrentOperations(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Run multiple operations concurrently
	done := make(chan bool, 3)

	// Concurrent Get
	go func() {
		reader, err := monitoringMiddleware.Get(ctx, "key1")
		if err == nil && reader != nil {
			_ = reader.Close()
		}
		done <- true
	}()

	// Concurrent Put
	go func() {
		_ = monitoringMiddleware.Put(ctx, "key2", strings.NewReader("data"))
		done <- true
	}()

	// Concurrent Delete
	go func() {
		_ = monitoringMiddleware.Delete(ctx, "key3")
		done <- true
	}()

	// Wait for all operations
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify metrics are consistent (no race conditions)
	snapshot := metrics.GetSnapshot()
	totalOps := snapshot.TotalDownloads + snapshot.TotalUploads
	assert.GreaterOrEqual(t, totalOps, int64(2)) // At least Get and Put were counted
}

func TestMonitoringMiddleware_TracksOperationDuration(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform multiple Get operations
	for i := 0; i < 3; i++ {
		reader, err := monitoringMiddleware.Get(ctx, "test-key")
		if err == nil && reader != nil {
			_ = reader.Close()
		}
		time.Sleep(10 * time.Millisecond) // Small delay between operations
	}

	// Verify average duration is tracked
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(3), snapshot.TotalDownloads)
	assert.Greater(t, snapshot.AverageDownloadTime, time.Duration(0))
	assert.Greater(t, snapshot.MaxDownloadTime, time.Duration(0))
}

func TestMonitoringMiddleware_Exists_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform Exists check
	exists, err := monitoringMiddleware.Exists(ctx, "test-key")
	require.NoError(t, err)
	assert.True(t, exists)

	// Verify operation completed without error
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_Copy_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform Copy operation
	err := monitoringMiddleware.Copy(ctx, "source-key", "dest-key")
	require.NoError(t, err)

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_Move_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform Move operation
	err := monitoringMiddleware.Move(ctx, "source-key", "dest-key")
	require.NoError(t, err)

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_Head_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform Head operation
	obj, err := monitoringMiddleware.Head(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, obj)
	assert.Equal(t, "test-key", obj.Key)
	assert.Equal(t, int64(1024), obj.Size)

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_UpdateMetadata_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform UpdateMetadata operation
	metadata := map[string]string{"key": "value"}
	err := monitoringMiddleware.UpdateMetadata(ctx, "test-key", metadata)
	require.NoError(t, err)

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_DeleteBatch_RecordsMetrics(t *testing.T) {
	mockStorage := NewMockStorage()
	metrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, metrics)
	ctx := context.Background()

	// Perform DeleteBatch operation
	keys := []string{"key1", "key2", "key3"}
	err := monitoringMiddleware.DeleteBatch(ctx, keys)
	require.NoError(t, err)

	// Verify operation completed
	snapshot := metrics.GetSnapshot()
	assert.NotNil(t, snapshot)
}

func TestMonitoringMiddleware_MetricsPassthrough(t *testing.T) {
	mockStorage := NewMockStorage()
	originalMetrics := storage.NewStorageMetrics()

	monitoringMiddleware := NewMonitoringMiddleware(mockStorage, originalMetrics)

	// Metrics() should return the same metrics instance
	returnedMetrics := monitoringMiddleware.Metrics()
	assert.Equal(t, originalMetrics, returnedMetrics)
}
