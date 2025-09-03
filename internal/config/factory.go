package config

import (
    "fmt"

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
    s, err := factory.New(cfg)
    if err != nil {
        return nil, fmt.Errorf("create storage from config: %w", err)
    }
    return s, nil
}
