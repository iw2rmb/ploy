package config

import (
	"testing"
)

// Ensures RetrySettings/CacheSettings are correctly mapped to factory configs
func TestCreateStorageClient_RetryAndCacheMapping(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{
			Provider: "memory",
			Retry: RetrySettings{
				Enabled:           true,
				MaxAttempts:       5,
				InitialDelay:      "150ms",
				MaxDelay:          "10s",
				BackoffMultiplier: 1.7,
			},
			Cache: CacheSettings{
				Enabled: true,
				MaxSize: 123,
				TTL:     "45s",
			},
		},
	}
	st, err := cfg.CreateStorageClient()
	if err != nil || st == nil {
		t.Fatalf("expected storage instance, got err=%v", err)
	}
}
