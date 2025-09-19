package storage

import "time"

// StorageClient provides comprehensive error handling, retry logic, and monitoring.
type StorageClient struct {
	client        StorageProvider
	retryClient   *RetryableStorageClient
	metrics       *StorageMetrics
	healthChecker *HealthChecker
	config        *ClientConfig
}

// ClientConfig configures the storage client.
type ClientConfig struct {
	RetryConfig       *RetryConfig       `json:"retry_config"`
	HealthCheckConfig *HealthCheckConfig `json:"health_check_config"`
	EnableMetrics     bool               `json:"enable_metrics"`
	EnableHealthCheck bool               `json:"enable_health_check"`
	MaxOperationTime  time.Duration      `json:"max_operation_time"`
}

// DefaultClientConfig returns sensible defaults for storage client.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		RetryConfig:       DefaultRetryConfig(),
		HealthCheckConfig: DefaultHealthCheckConfig(),
		EnableMetrics:     true,
		EnableHealthCheck: true,
		MaxOperationTime:  5 * time.Minute,
	}
}

// NewStorageClient creates a new storage client with comprehensive error handling.
func NewStorageClient(client StorageProvider, config *ClientConfig) *StorageClient {
	if config == nil {
		config = DefaultClientConfig()
	}

	enhanced := &StorageClient{
		client: client,
		config: config,
	}

	enhanced.retryClient = NewRetryableStorageClient(client, config.RetryConfig)

	if config.EnableMetrics {
		enhanced.metrics = NewStorageMetrics()
	}

	if config.EnableHealthCheck && enhanced.metrics != nil {
		enhanced.healthChecker = NewHealthChecker(client, enhanced.metrics, config.HealthCheckConfig)
	}

	return enhanced
}
