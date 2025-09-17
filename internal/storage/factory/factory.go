package factory

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/middleware"
	"github.com/iw2rmb/ploy/internal/storage/providers/memory"
	"github.com/iw2rmb/ploy/internal/storage/providers/seaweedfs"
)

// FactoryConfig defines the configuration for creating a storage instance
type FactoryConfig struct {
	Provider   string                 // "seaweedfs", "s3", "memory"
	Endpoint   string                 // Provider endpoint
	Bucket     string                 // Default bucket/collection
	Region     string                 // For S3
	Retry      RetryConfig            // Retry configuration
	Monitoring MonitoringConfig       // Monitoring configuration
	Cache      CacheConfig            // Cache configuration
	Extra      map[string]interface{} // Provider-specific config
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	Enabled           bool
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
}

// MonitoringConfig defines monitoring behavior
type MonitoringConfig struct {
	Enabled bool
}

// CacheConfig defines caching behavior
type CacheConfig struct {
	Enabled bool
	MaxSize int           // Maximum number of cached items
	TTL     time.Duration // Time to live for cached items
}

// New creates a new storage instance with the specified configuration
func New(cfg FactoryConfig) (storage.Storage, error) {
	// Default to seaweedfs if no provider specified
	if cfg.Provider == "" {
		cfg.Provider = "seaweedfs"
	}

	// Create base provider
	var base storage.Storage
	var err error

	switch cfg.Provider {
	case "seaweedfs":
		base, err = createSeaweedFSProvider(cfg)
	case "s3":
		base, err = createS3Provider(cfg)
	case "memory":
		base = createMemoryProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, err
	}

	// Apply middleware layers in order: cache -> monitoring -> retry
	storage := base

	// Apply retry layer (innermost middleware after base)
	if cfg.Retry.Enabled {
		storage = applyRetryMiddleware(storage, cfg.Retry)
	}

	// Apply monitoring layer
	if cfg.Monitoring.Enabled {
		storage = applyMonitoringMiddleware(storage)
	}

	// Apply cache layer (outermost middleware)
	if cfg.Cache.Enabled {
		storage = applyCacheMiddleware(storage, cfg.Cache)
	}

	return storage, nil
}

// createSeaweedFSProvider creates a SeaweedFS storage provider
func createSeaweedFSProvider(cfg FactoryConfig) (storage.Storage, error) {
	// Require endpoint for unit tests and explicit config; avoid magic defaults here.
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("endpoint required for seaweedfs provider")
	}

	masterURL := cfg.Endpoint

	// Filer can still be overridden via env/extra, otherwise use a local default suitable for tests
	filerURL := os.Getenv("SEAWEEDFS_FILER")
	if filerURL == "" {
		filerURL = "http://localhost:8888"
	}

	seaweedCfg := seaweedfs.Config{
		Master:      masterURL,
		Filer:       filerURL,
		Collection:  cfg.Bucket,
		Replication: "000", // Default replication (000 for single-node dev)
		Timeout:     30,
	}

	// Apply extra configuration if provided
	if cfg.Extra != nil {
		if filer, ok := cfg.Extra["filer"].(string); ok {
			seaweedCfg.Filer = filer
		}
		if replication, ok := cfg.Extra["replication"].(string); ok {
			seaweedCfg.Replication = replication
		}
		if timeout, ok := cfg.Extra["timeout"].(int); ok {
			seaweedCfg.Timeout = timeout
		}
	}

	client, err := seaweedfs.New(seaweedCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create seaweedfs provider: %w", err)
	}

	// SeaweedFS provider already implements Storage interface
	return client, nil
}

// createS3Provider creates an S3 storage provider
func createS3Provider(cfg FactoryConfig) (storage.Storage, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("region required for s3 provider")
	}

	// S3 provider implementation would go here
	// For now, return an error as S3 is not yet implemented
	return nil, fmt.Errorf("s3 provider not yet implemented")
}

// createMemoryProvider creates an in-memory storage provider
func createMemoryProvider(cfg FactoryConfig) storage.Storage {
	// Check for max memory limit in extra config
	maxMemory := int64(0)
	if cfg.Extra != nil {
		if limit, ok := cfg.Extra["maxMemory"].(int); ok {
			maxMemory = int64(limit)
		}
	}

	return memory.NewMemoryStorage(maxMemory)
}

// applyRetryMiddleware wraps the storage with retry logic
func applyRetryMiddleware(stor storage.Storage, cfg RetryConfig) storage.Storage {
	retryCfg := &middleware.RetryConfig{
		MaxAttempts:       cfg.MaxAttempts,
		InitialDelay:      cfg.InitialDelay,
		MaxDelay:          cfg.MaxDelay,
		BackoffMultiplier: cfg.BackoffMultiplier,
	}

	// Set defaults if not specified
	if retryCfg.MaxAttempts == 0 {
		retryCfg.MaxAttempts = 3
	}
	if retryCfg.InitialDelay == 0 {
		retryCfg.InitialDelay = 100 * time.Millisecond
	}
	if retryCfg.MaxDelay == 0 {
		retryCfg.MaxDelay = 30 * time.Second
	}
	if retryCfg.BackoffMultiplier == 0 {
		retryCfg.BackoffMultiplier = 2.0
	}

	// Set default retry function if not provided
	if retryCfg.ShouldRetry == nil {
		retryCfg.ShouldRetry = func(err *storage.StorageError, attempt int) bool {
			if attempt >= retryCfg.MaxAttempts {
				return false
			}
			return err != nil && err.Retryable
		}
	}

	return middleware.NewRetryMiddleware(stor, retryCfg)
}

// applyMonitoringMiddleware wraps the storage with monitoring
func applyMonitoringMiddleware(stor storage.Storage) storage.Storage {
	metrics := storage.NewStorageMetrics()
	return middleware.NewMonitoringMiddleware(stor, metrics)
}

// applyCacheMiddleware wraps the storage with caching
func applyCacheMiddleware(stor storage.Storage, cfg CacheConfig) storage.Storage {
	cacheCfg := &middleware.CacheConfig{
		MaxSize: cfg.MaxSize,
		TTL:     cfg.TTL,
	}

	// Set defaults if not specified
	if cacheCfg.MaxSize == 0 {
		cacheCfg.MaxSize = 100
	}
	if cacheCfg.TTL == 0 {
		cacheCfg.TTL = 5 * time.Minute
	}

	return middleware.NewCacheMiddleware(stor, cacheCfg)
}
