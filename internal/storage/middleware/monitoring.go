package middleware

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// MonitoringMiddleware implements storage.Storage with metrics collection
type MonitoringMiddleware struct {
	next    storage.Storage
	metrics *storage.StorageMetrics
}

// NewMonitoringMiddleware creates a new monitoring middleware
func NewMonitoringMiddleware(next storage.Storage, metrics *storage.StorageMetrics) *MonitoringMiddleware {
	if metrics == nil {
		metrics = storage.NewStorageMetrics()
	}

	return &MonitoringMiddleware{
		next:    next,
		metrics: metrics,
	}
}

// extractErrorType extracts the error type from a storage error
func extractErrorType(err error) storage.ErrorType {
	if err == nil {
		return ""
	}

	if storageErr, ok := err.(*storage.StorageError); ok {
		return storageErr.ErrorType
	}

	return storage.ErrorTypeInternal // Default for unknown errors
}

// getReaderSize attempts to get the size of a reader without consuming it
func getReaderSize(reader io.Reader) int64 {
	if reader == nil {
		return 0
	}

	// Try to get size without consuming the reader
	switch r := reader.(type) {
	case *strings.Reader:
		return r.Size()
	default:
		// For other reader types, estimate or use 0
		return 0
	}
}

// Get retrieves an object and records metrics
func (m *MonitoringMiddleware) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	start := time.Now()

	reader, err := m.next.Get(ctx, key)
	duration := time.Since(start)

	// Record download metrics
	if err == nil {
		// For successful downloads, we don't know the size until we read
		// So we record with size 0 for now
		m.metrics.RecordDownload(true, duration, 0, "")
	} else {
		m.metrics.RecordDownload(false, duration, 0, extractErrorType(err))
	}

	return reader, err
}

// Put stores an object and records metrics
func (m *MonitoringMiddleware) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	start := time.Now()
	size := getReaderSize(reader)

	err := m.next.Put(ctx, key, reader, opts...)
	duration := time.Since(start)

	// Record upload metrics
	if err == nil {
		m.metrics.RecordUpload(true, duration, size, "")
	} else {
		m.metrics.RecordUpload(false, duration, 0, extractErrorType(err))
	}

	return err
}

// Delete removes an object and records metrics
func (m *MonitoringMiddleware) Delete(ctx context.Context, key string) error {
	start := time.Now()

	err := m.next.Delete(ctx, key)
	duration := time.Since(start)

	// Record as a generic operation (delete doesn't have specific metrics)
	if err != nil {
		errorType := extractErrorType(err)
		// Record the error in metrics
		m.metrics.RecordDownload(false, duration, 0, errorType)
	}

	return err
}

// Exists checks if an object exists and records metrics
func (m *MonitoringMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()

	exists, err := m.next.Exists(ctx, key)
	duration := time.Since(start)

	// Record as a verification operation
	if err == nil {
		m.metrics.RecordVerification(true, "")
	} else {
		m.metrics.RecordVerification(false, extractErrorType(err))
	}

	_ = duration // Duration tracked but not used for verification

	return exists, err
}

// List lists objects and records metrics
func (m *MonitoringMiddleware) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	start := time.Now()

	objects, err := m.next.List(ctx, opts)
	duration := time.Since(start)

	// Record as a download operation (listing is a read operation)
	if err == nil {
		m.metrics.RecordDownload(true, duration, 0, "")
	} else {
		m.metrics.RecordDownload(false, duration, 0, extractErrorType(err))
	}

	return objects, err
}

// DeleteBatch deletes multiple objects and records metrics
func (m *MonitoringMiddleware) DeleteBatch(ctx context.Context, keys []string) error {
	start := time.Now()

	err := m.next.DeleteBatch(ctx, keys)
	duration := time.Since(start)

	// Record as multiple delete operations
	if err != nil {
		errorType := extractErrorType(err)
		// Record one failure for the batch
		m.metrics.RecordUpload(false, duration, 0, errorType)
	}

	return err
}

// Head gets object metadata and records metrics
func (m *MonitoringMiddleware) Head(ctx context.Context, key string) (*storage.Object, error) {
	start := time.Now()

	obj, err := m.next.Head(ctx, key)
	duration := time.Since(start)

	// Record as a verification operation
	if err == nil {
		m.metrics.RecordVerification(true, "")
	} else {
		m.metrics.RecordVerification(false, extractErrorType(err))
	}

	_ = duration // Duration tracked but not used for verification

	return obj, err
}

// UpdateMetadata updates object metadata and records metrics
func (m *MonitoringMiddleware) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	start := time.Now()

	err := m.next.UpdateMetadata(ctx, key, metadata)
	duration := time.Since(start)

	// Record as an upload operation (metadata update is a write)
	if err == nil {
		m.metrics.RecordUpload(true, duration, 0, "")
	} else {
		m.metrics.RecordUpload(false, duration, 0, extractErrorType(err))
	}

	return err
}

// Copy copies an object and records metrics
func (m *MonitoringMiddleware) Copy(ctx context.Context, src, dst string) error {
	start := time.Now()

	err := m.next.Copy(ctx, src, dst)
	duration := time.Since(start)

	// Record as an upload operation (copy creates a new object)
	if err == nil {
		m.metrics.RecordUpload(true, duration, 0, "")
	} else {
		m.metrics.RecordUpload(false, duration, 0, extractErrorType(err))
	}

	return err
}

// Move moves an object and records metrics
func (m *MonitoringMiddleware) Move(ctx context.Context, src, dst string) error {
	start := time.Now()

	err := m.next.Move(ctx, src, dst)
	duration := time.Since(start)

	// Record as an upload operation (move is like copy + delete)
	if err == nil {
		m.metrics.RecordUpload(true, duration, 0, "")
	} else {
		m.metrics.RecordUpload(false, duration, 0, extractErrorType(err))
	}

	return err
}

// Health checks storage health and records metrics
func (m *MonitoringMiddleware) Health(ctx context.Context) error {
	start := time.Now()

	err := m.next.Health(ctx)
	duration := time.Since(start)

	// Record as a verification operation
	if err == nil {
		m.metrics.RecordVerification(true, "")
		// Update health status to healthy if check passes
		snapshot := m.metrics.GetSnapshot()
		if snapshot.HealthStatus == storage.HealthStatusUnknown {
			// Initialize to healthy if unknown
			m.metrics.RecordVerification(true, "")
		}
	} else {
		m.metrics.RecordVerification(false, extractErrorType(err))
	}

	_ = duration // Duration tracked but not used for health checks

	return err
}

// Metrics returns the storage metrics instance
func (m *MonitoringMiddleware) Metrics() *storage.StorageMetrics {
	return m.metrics
}
