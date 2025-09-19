package storage

import (
	"encoding/json"
	"sync"
	"time"
)

// StorageMetrics tracks comprehensive storage operation metrics.
type StorageMetrics struct {
	TotalUploads            int64 `json:"total_uploads"`
	SuccessfulUploads       int64 `json:"successful_uploads"`
	FailedUploads           int64 `json:"failed_uploads"`
	TotalDownloads          int64 `json:"total_downloads"`
	SuccessfulDownloads     int64 `json:"successful_downloads"`
	FailedDownloads         int64 `json:"failed_downloads"`
	TotalVerifications      int64 `json:"total_verifications"`
	SuccessfulVerifications int64 `json:"successful_verifications"`
	FailedVerifications     int64 `json:"failed_verifications"`

	TotalBytesUploaded   int64 `json:"total_bytes_uploaded"`
	TotalBytesDownloaded int64 `json:"total_bytes_downloaded"`

	AverageUploadTime   time.Duration `json:"average_upload_time"`
	AverageDownloadTime time.Duration `json:"average_download_time"`
	MaxUploadTime       time.Duration `json:"max_upload_time"`
	MaxDownloadTime     time.Duration `json:"max_download_time"`

	ErrorsByType  map[ErrorType]int64 `json:"errors_by_type"`
	RetryAttempts int64               `json:"retry_attempts"`

	LastSuccessfulOperation time.Time    `json:"last_successful_operation"`
	ConsecutiveFailures     int64        `json:"consecutive_failures"`
	HealthStatus            HealthStatus `json:"health_status"`

	mutex sync.RWMutex `json:"-"`
}

// HealthStatus represents the overall health of storage operations.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// NewStorageMetrics creates a new metrics instance.
func NewStorageMetrics() *StorageMetrics {
	return &StorageMetrics{
		ErrorsByType: make(map[ErrorType]int64),
		HealthStatus: HealthStatusUnknown,
	}
}

func (m *StorageMetrics) RecordUpload(success bool, duration time.Duration, bytes int64, errorType ErrorType) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalUploads++

	if success {
		m.SuccessfulUploads++
		m.TotalBytesUploaded += bytes
		m.LastSuccessfulOperation = time.Now()
		m.ConsecutiveFailures = 0

		if m.SuccessfulUploads == 1 {
			m.AverageUploadTime = duration
		} else {
			m.AverageUploadTime = time.Duration((int64(m.AverageUploadTime)*(m.SuccessfulUploads-1) + int64(duration)) / m.SuccessfulUploads)
		}

		if duration > m.MaxUploadTime {
			m.MaxUploadTime = duration
		}
	} else {
		m.FailedUploads++
		m.ConsecutiveFailures++
		if errorType != "" {
			m.ErrorsByType[errorType]++
		}
	}

	m.updateHealthStatus()
}

func (m *StorageMetrics) RecordDownload(success bool, duration time.Duration, bytes int64, errorType ErrorType) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalDownloads++

	if success {
		m.SuccessfulDownloads++
		m.TotalBytesDownloaded += bytes
		m.LastSuccessfulOperation = time.Now()
		m.ConsecutiveFailures = 0

		if m.SuccessfulDownloads == 1 {
			m.AverageDownloadTime = duration
		} else {
			m.AverageDownloadTime = time.Duration((int64(m.AverageDownloadTime)*(m.SuccessfulDownloads-1) + int64(duration)) / m.SuccessfulDownloads)
		}

		if duration > m.MaxDownloadTime {
			m.MaxDownloadTime = duration
		}
	} else {
		m.FailedDownloads++
		m.ConsecutiveFailures++
		if errorType != "" {
			m.ErrorsByType[errorType]++
		}
	}

	m.updateHealthStatus()
}

func (m *StorageMetrics) RecordVerification(success bool, errorType ErrorType) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalVerifications++

	if success {
		m.SuccessfulVerifications++
		m.LastSuccessfulOperation = time.Now()
		m.ConsecutiveFailures = 0
	} else {
		m.FailedVerifications++
		m.ConsecutiveFailures++
		if errorType != "" {
			m.ErrorsByType[errorType]++
		}
	}

	m.updateHealthStatus()
}

func (m *StorageMetrics) RecordRetry() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.RetryAttempts++
}

func (m *StorageMetrics) updateHealthStatus() {
	totalOps := m.TotalUploads + m.TotalDownloads + m.TotalVerifications
	totalFailures := m.FailedUploads + m.FailedDownloads + m.FailedVerifications

	if totalOps == 0 {
		m.HealthStatus = HealthStatusUnknown
		return
	}

	failureRate := float64(totalFailures) / float64(totalOps)
	timeSinceSuccess := time.Since(m.LastSuccessfulOperation)

	switch {
	case m.ConsecutiveFailures >= 10 || failureRate > 0.5 || timeSinceSuccess > 10*time.Minute:
		m.HealthStatus = HealthStatusUnhealthy
	case m.ConsecutiveFailures >= 3 || failureRate > 0.1 || timeSinceSuccess > 5*time.Minute:
		m.HealthStatus = HealthStatusDegraded
	default:
		m.HealthStatus = HealthStatusHealthy
	}
}

func (m *StorageMetrics) GetSnapshot() *StorageMetrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	snapshot := &StorageMetrics{
		TotalUploads:            m.TotalUploads,
		SuccessfulUploads:       m.SuccessfulUploads,
		FailedUploads:           m.FailedUploads,
		TotalDownloads:          m.TotalDownloads,
		SuccessfulDownloads:     m.SuccessfulDownloads,
		FailedDownloads:         m.FailedDownloads,
		TotalVerifications:      m.TotalVerifications,
		SuccessfulVerifications: m.SuccessfulVerifications,
		FailedVerifications:     m.FailedVerifications,
		TotalBytesUploaded:      m.TotalBytesUploaded,
		TotalBytesDownloaded:    m.TotalBytesDownloaded,
		AverageUploadTime:       m.AverageUploadTime,
		AverageDownloadTime:     m.AverageDownloadTime,
		MaxUploadTime:           m.MaxUploadTime,
		MaxDownloadTime:         m.MaxDownloadTime,
		ErrorsByType:            make(map[ErrorType]int64),
		RetryAttempts:           m.RetryAttempts,
		LastSuccessfulOperation: m.LastSuccessfulOperation,
		ConsecutiveFailures:     m.ConsecutiveFailures,
		HealthStatus:            m.HealthStatus,
	}

	for k, v := range m.ErrorsByType {
		snapshot.ErrorsByType[k] = v
	}
	return snapshot
}

func (m *StorageMetrics) GetSuccessRate() float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalOps := m.TotalUploads + m.TotalDownloads + m.TotalVerifications
	if totalOps == 0 {
		return 0
	}
	successfulOps := m.SuccessfulUploads + m.SuccessfulDownloads + m.SuccessfulVerifications
	return (float64(successfulOps) / float64(totalOps)) * 100
}

func (m *StorageMetrics) ToJSON() ([]byte, error) {
	snapshot := m.GetSnapshot()
	return json.MarshalIndent(snapshot, "", "  ")
}
