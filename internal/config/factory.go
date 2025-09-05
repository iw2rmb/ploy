package config

import (
	"fmt"
	"time"

	storage "github.com/iw2rmb/ploy/internal/storage"
	factory "github.com/iw2rmb/ploy/internal/storage/factory"
)

// CreateStorageClient constructs a storage.Storage from this Config.
// Minimal implementation: supports memory provider for this slice.
func (c *Config) CreateStorageClient() (storage.Storage, error) {
	provider := c.Storage.Provider
	if provider == "" {
		// default to memory for tests and safety
		provider = "memory"
	}
	cfg := factory.FactoryConfig{
		Provider: provider,
		Endpoint: c.Storage.Endpoint,
		Bucket:   c.Storage.Bucket,
		Region:   c.Storage.Region,
	}
	// Map retry settings if enabled
	if c.Storage.Retry.Enabled {
		var initDelay, maxDelay time.Duration
		if d, err := time.ParseDuration(c.Storage.Retry.InitialDelay); err == nil {
			initDelay = d
		}
		if d, err := time.ParseDuration(c.Storage.Retry.MaxDelay); err == nil {
			maxDelay = d
		}
		cfg.Retry = factory.RetryConfig{
			Enabled:           true,
			MaxAttempts:       c.Storage.Retry.MaxAttempts,
			InitialDelay:      initDelay,
			MaxDelay:          maxDelay,
			BackoffMultiplier: c.Storage.Retry.BackoffMultiplier,
		}
	}
	// Map cache settings if enabled
	if c.Storage.Cache.Enabled {
		var ttl time.Duration
		if d, err := time.ParseDuration(c.Storage.Cache.TTL); err == nil {
			ttl = d
		}
		cfg.Cache = factory.CacheConfig{
			Enabled: true,
			MaxSize: c.Storage.Cache.MaxSize,
			TTL:     ttl,
		}
	}
	s, err := factory.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create storage from config: %w", err)
	}
	return s, nil
}
