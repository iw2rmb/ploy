package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StorageMetrics tracks comprehensive storage operation metrics
type StorageMetrics struct {
	// Operation counters
	TotalUploads            int64 `json:"total_uploads"`
	SuccessfulUploads       int64 `json:"successful_uploads"`
	FailedUploads           int64 `json:"failed_uploads"`
	TotalDownloads          int64 `json:"total_downloads"`
	SuccessfulDownloads     int64 `json:"successful_downloads"`
	FailedDownloads         int64 `json:"failed_downloads"`
	TotalVerifications      int64 `json:"total_verifications"`
	SuccessfulVerifications int64 `json:"successful_verifications"`
	FailedVerifications     int64 `json:"failed_verifications"`

	// Size metrics
	TotalBytesUploaded   int64 `json:"total_bytes_uploaded"`
	TotalBytesDownloaded int64 `json:"total_bytes_downloaded"`

	// Performance metrics
	AverageUploadTime   time.Duration `json:"average_upload_time"`
	AverageDownloadTime time.Duration `json:"average_download_time"`
	MaxUploadTime       time.Duration `json:"max_upload_time"`
	MaxDownloadTime     time.Duration `json:"max_download_time"`

	// Error tracking
	ErrorsByType  map[ErrorType]int64 `json:"errors_by_type"`
	RetryAttempts int64               `json:"retry_attempts"`

	// Health status
	LastSuccessfulOperation time.Time    `json:"last_successful_operation"`
	ConsecutiveFailures     int64        `json:"consecutive_failures"`
	HealthStatus            HealthStatus `json:"health_status"`

	// Lock for thread safety
	mutex sync.RWMutex `json:"-"`
}

// HealthStatus represents the overall health of storage operations
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// NewStorageMetrics creates a new metrics instance
func NewStorageMetrics() *StorageMetrics {
	return &StorageMetrics{
		ErrorsByType: make(map[ErrorType]int64),
		HealthStatus: HealthStatusUnknown,
	}
}

// RecordUpload records an upload operation
func (m *StorageMetrics) RecordUpload(success bool, duration time.Duration, bytes int64, errorType ErrorType) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalUploads++

	if success {
		m.SuccessfulUploads++
		m.TotalBytesUploaded += bytes
		m.LastSuccessfulOperation = time.Now()
		m.ConsecutiveFailures = 0

		// Update average upload time
		if m.SuccessfulUploads == 1 {
			m.AverageUploadTime = duration
		} else {
			m.AverageUploadTime = time.Duration(
				(int64(m.AverageUploadTime)*(m.SuccessfulUploads-1) + int64(duration)) / m.SuccessfulUploads)
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

// RecordDownload records a download operation
func (m *StorageMetrics) RecordDownload(success bool, duration time.Duration, bytes int64, errorType ErrorType) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalDownloads++

	if success {
		m.SuccessfulDownloads++
		m.TotalBytesDownloaded += bytes
		m.LastSuccessfulOperation = time.Now()
		m.ConsecutiveFailures = 0

		// Update average download time
		if m.SuccessfulDownloads == 1 {
			m.AverageDownloadTime = duration
		} else {
			m.AverageDownloadTime = time.Duration(
				(int64(m.AverageDownloadTime)*(m.SuccessfulDownloads-1) + int64(duration)) / m.SuccessfulDownloads)
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

// RecordVerification records a verification operation
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

// RecordRetry records a retry attempt
func (m *StorageMetrics) RecordRetry() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.RetryAttempts++
}

// updateHealthStatus updates the overall health status based on current metrics
func (m *StorageMetrics) updateHealthStatus() {
	// Calculate failure rates
	totalOps := m.TotalUploads + m.TotalDownloads + m.TotalVerifications
	totalFailures := m.FailedUploads + m.FailedDownloads + m.FailedVerifications

	if totalOps == 0 {
		m.HealthStatus = HealthStatusUnknown
		return
	}

	failureRate := float64(totalFailures) / float64(totalOps)

	// Check time since last successful operation
	timeSinceSuccess := time.Since(m.LastSuccessfulOperation)

	// Determine health status
	if m.ConsecutiveFailures >= 10 || failureRate > 0.5 || timeSinceSuccess > 10*time.Minute {
		m.HealthStatus = HealthStatusUnhealthy
	} else if m.ConsecutiveFailures >= 3 || failureRate > 0.1 || timeSinceSuccess > 5*time.Minute {
		m.HealthStatus = HealthStatusDegraded
	} else {
		m.HealthStatus = HealthStatusHealthy
	}
}

// GetSnapshot returns a thread-safe snapshot of current metrics
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

	// Copy error map
	for k, v := range m.ErrorsByType {
		snapshot.ErrorsByType[k] = v
	}

	return snapshot
}

// GetSuccessRate returns the overall success rate as a percentage
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

// ToJSON returns a JSON representation of the metrics
func (m *StorageMetrics) ToJSON() ([]byte, error) {
	snapshot := m.GetSnapshot()
	return json.MarshalIndent(snapshot, "", "  ")
}

// HealthChecker provides comprehensive storage health checking
type HealthChecker struct {
	client  StorageProvider
	metrics *StorageMetrics
	config  *HealthCheckConfig
}

// HealthCheckConfig configures health check behavior
type HealthCheckConfig struct {
	CheckInterval   time.Duration `json:"check_interval"`
	Timeout         time.Duration `json:"timeout"`
	TestBucket      string        `json:"test_bucket"`
	TestObjectSize  int64         `json:"test_object_size"`
	EnableDeepCheck bool          `json:"enable_deep_check"`
}

// DefaultHealthCheckConfig returns default health check configuration
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		CheckInterval:   5 * time.Minute,
		Timeout:         30 * time.Second,
		TestBucket:      "ploy-health-check",
		TestObjectSize:  1024, // 1KB test object
		EnableDeepCheck: true,
	}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(client StorageProvider, metrics *StorageMetrics, config *HealthCheckConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	return &HealthChecker{
		client:  client,
		metrics: metrics,
		config:  config,
	}
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Timestamp    time.Time              `json:"timestamp"`
	Status       HealthStatus           `json:"status"`
	ResponseTime time.Duration          `json:"response_time"`
	Checks       map[string]CheckResult `json:"checks"`
	Summary      string                 `json:"summary"`
	Metrics      *StorageMetrics        `json:"metrics,omitempty"`
}

// CheckResult represents the result of an individual check
type CheckResult struct {
	Status   HealthStatus  `json:"status"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// PerformHealthCheck executes a comprehensive health check
func (h *HealthChecker) PerformHealthCheck(ctx context.Context) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		Timestamp: start,
		Status:    HealthStatusHealthy,
		Checks:    make(map[string]CheckResult),
	}

	// Basic connectivity check
	h.checkConnectivity(ctx, result)

	// Configuration validation
	h.checkConfiguration(ctx, result)

	if h.config.EnableDeepCheck {
		// Deep storage operations check
		h.checkStorageOperations(ctx, result)
	}

	// Include current metrics
	result.Metrics = h.metrics.GetSnapshot()

	// Calculate overall status and response time
	result.ResponseTime = time.Since(start)
	h.determineOverallStatus(result)

	return result
}

// checkConnectivity verifies basic connectivity to storage service
func (h *HealthChecker) checkConnectivity(ctx context.Context, result *HealthCheckResult) {
	start := time.Now()
	checkResult := CheckResult{
		Status: HealthStatusHealthy,
	}

	// For SeaweedFS, test filer connectivity directly instead of listing objects
	if seaweedClient, ok := h.client.(*SeaweedFSClient); ok {
		// Test filer root directory access (lightweight HTTP request)
		_, err := h.client.ListObjects("", "")
		duration := time.Since(start)
		checkResult.Duration = duration

		if err != nil {
			// Try volume assignment as fallback connectivity test
			if _, assignErr := seaweedClient.TestVolumeAssignment(); assignErr != nil {
				checkResult.Status = HealthStatusUnhealthy
				checkResult.Message = "SeaweedFS services unreachable"
				checkResult.Error = err.Error()
				result.Status = HealthStatusUnhealthy
			} else {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Master reachable, filer may have directory issues"
				checkResult.Error = err.Error()
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			}
		} else {
			checkResult.Message = fmt.Sprintf("SeaweedFS services responsive (%.2fms)",
				float64(duration.Nanoseconds())/1e6)
		}
	} else {
		// For other storage providers, try listing objects in root
		_, err := h.client.ListObjects("", "")
		duration := time.Since(start)
		checkResult.Duration = duration

		if err != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Storage service unreachable"
			checkResult.Error = err.Error()
			result.Status = HealthStatusUnhealthy
		} else {
			checkResult.Message = fmt.Sprintf("Storage service responsive (%.2fms)",
				float64(duration.Nanoseconds())/1e6)
		}
	}

	result.Checks["connectivity"] = checkResult
}

// checkConfiguration validates storage configuration
func (h *HealthChecker) checkConfiguration(ctx context.Context, result *HealthCheckResult) {
	checkResult := CheckResult{
		Status: HealthStatusHealthy,
	}

	// Validate provider type
	providerType := h.client.GetProviderType()
	if providerType == "" {
		checkResult.Status = HealthStatusUnhealthy
		checkResult.Message = "Storage provider type not configured"
	} else {
		// Validate bucket configuration
		bucket := h.client.GetArtifactsBucket()
		if bucket == "" {
			checkResult.Status = HealthStatusDegraded
			checkResult.Message = "Artifacts bucket not configured"
		} else {
			checkResult.Message = fmt.Sprintf("Configuration valid (provider: %s, bucket: %s)",
				providerType, bucket)
		}
	}

	if checkResult.Status != HealthStatusHealthy && result.Status == HealthStatusHealthy {
		result.Status = checkResult.Status
	}

	result.Checks["configuration"] = checkResult
}

// checkStorageOperations performs deep storage operation testing
func (h *HealthChecker) checkStorageOperations(ctx context.Context, result *HealthCheckResult) {
	start := time.Now()
	checkResult := CheckResult{
		Status: HealthStatusHealthy,
	}

	// For SeaweedFS, use volume assignment testing instead of full upload/download
	// This avoids filer directory creation issues while still testing core functionality
	bucket := h.client.GetArtifactsBucket()
	if seaweedClient, ok := h.client.(*SeaweedFSClient); ok {
		// Test volume assignment first (this is the core SeaweedFS operation)
		assignment, assignErr := seaweedClient.TestVolumeAssignment()
		if assignErr != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Volume assignment failed - storage unavailable"
			checkResult.Error = assignErr.Error()
			result.Status = HealthStatusUnhealthy
		} else {
			// Volume assignment successful - verify we got valid assignment
			if fid, ok := assignment["fid"].(string); ok && fid != "" {
				if url, ok := assignment["url"].(string); ok && url != "" {
					checkResult.Message = fmt.Sprintf("Volume assignment successful (FID: %s, URL: %s)", fid, url)

					// Optional: Test a lightweight upload operation with simplified key (no directories)
					testKey := fmt.Sprintf("health_%d", time.Now().Unix())
					testData := strings.NewReader("healthcheck")

					if _, uploadErr := h.client.PutObject(bucket, testKey, testData, "text/plain"); uploadErr != nil {
						// Volume assignment works but upload fails - degraded state
						if strings.Contains(uploadErr.Error(), "409 Conflict") || strings.Contains(uploadErr.Error(), "failed to create directory") {
							checkResult.Status = HealthStatusDegraded
							checkResult.Message = "Volume assignment working, directory creation issues (expected for SeaweedFS)"
						} else {
							checkResult.Status = HealthStatusDegraded
							checkResult.Message = fmt.Sprintf("Volume assignment working, upload failed: %s", uploadErr.Error())
						}
						if result.Status == HealthStatusHealthy {
							result.Status = HealthStatusDegraded
						}
					} else {
						// Full success - both assignment and upload work
						duration := time.Since(start)
						checkResult.Message = fmt.Sprintf("Storage operations fully successful (%.2fms)",
							float64(duration.Nanoseconds())/1e6)
					}
				} else {
					checkResult.Status = HealthStatusDegraded
					checkResult.Message = "Volume assignment incomplete - missing URL"
					if result.Status == HealthStatusHealthy {
						result.Status = HealthStatusDegraded
					}
				}
			} else {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Volume assignment incomplete - missing File ID"
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			}
		}
	} else {
		// For non-SeaweedFS providers, use standard upload/download test
		testKey := fmt.Sprintf("health_%d.txt", time.Now().Unix())
		testData := strings.NewReader(strings.Repeat("A", int(h.config.TestObjectSize)))

		_, uploadErr := h.client.PutObject(bucket, testKey, testData, "text/plain")
		if uploadErr != nil {
			checkResult.Status = HealthStatusUnhealthy
			checkResult.Message = "Upload operation failed"
			checkResult.Error = uploadErr.Error()
			result.Status = HealthStatusUnhealthy
		} else {
			// Test download
			reader, downloadErr := h.client.GetObject(bucket, testKey)
			if downloadErr != nil {
				checkResult.Status = HealthStatusDegraded
				checkResult.Message = "Download operation failed"
				checkResult.Error = downloadErr.Error()
				if result.Status == HealthStatusHealthy {
					result.Status = HealthStatusDegraded
				}
			} else {
				reader.Close()
				duration := time.Since(start)
				checkResult.Message = fmt.Sprintf("Storage operations successful (%.2fms)",
					float64(duration.Nanoseconds())/1e6)
			}
		}
	}

	checkResult.Duration = time.Since(start)
	result.Checks["storage_operations"] = checkResult
}

// determineOverallStatus determines the overall health status
func (h *HealthChecker) determineOverallStatus(result *HealthCheckResult) {
	var messages []string

	for checkName, check := range result.Checks {
		messages = append(messages, fmt.Sprintf("%s: %s", checkName, check.Message))

		// Overall status is the worst of all checks
		if check.Status == HealthStatusUnhealthy {
			result.Status = HealthStatusUnhealthy
		} else if check.Status == HealthStatusDegraded && result.Status == HealthStatusHealthy {
			result.Status = HealthStatusDegraded
		}
	}

	// Include metrics-based status
	metricsStatus := result.Metrics.HealthStatus
	if metricsStatus == HealthStatusUnhealthy {
		result.Status = HealthStatusUnhealthy
	} else if metricsStatus == HealthStatusDegraded && result.Status == HealthStatusHealthy {
		result.Status = HealthStatusDegraded
	}

	result.Summary = strings.Join(messages, "; ")

	// Add overall assessment
	switch result.Status {
	case HealthStatusHealthy:
		result.Summary = "Storage system is healthy. " + result.Summary
	case HealthStatusDegraded:
		result.Summary = "Storage system is degraded. " + result.Summary
	case HealthStatusUnhealthy:
		result.Summary = "Storage system is unhealthy. " + result.Summary
	}
}
