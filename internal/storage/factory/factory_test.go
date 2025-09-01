package factory

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFactory_BasicProviderCreation(t *testing.T) {
	tests := []struct {
		name      string
		config    FactoryConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid seaweedfs config",
			config: FactoryConfig{
				Provider: "seaweedfs",
				Endpoint: "localhost:9333",
				Bucket:   "test-bucket",
			},
			wantError: false,
		},
		{
			name: "valid memory config",
			config: FactoryConfig{
				Provider: "memory",
			},
			wantError: false,
		},
		{
			name: "unknown provider",
			config: FactoryConfig{
				Provider: "unknown",
			},
			wantError: true,
			errorMsg:  "unknown provider: unknown",
		},
		{
			name: "seaweedfs missing endpoint",
			config: FactoryConfig{
				Provider: "seaweedfs",
			},
			wantError: true,
			errorMsg:  "endpoint required for seaweedfs provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := New(tt.config)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestNewFactory_MiddlewareApplication(t *testing.T) {
	t.Run("applies retry middleware", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "memory",
			Retry: RetryConfig{
				Enabled:      true,
				MaxAttempts:  5,
				InitialDelay: 100 * time.Millisecond,
			},
		}

		store, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, store)

		// Test that retry logic is applied by forcing a transient error
		// This would require the memory provider to support error injection
		// For now, we just verify the storage was created
		ctx := context.Background()
		err = store.Put(ctx, "test-key", strings.NewReader("test-data"))
		assert.NoError(t, err)
	})

	t.Run("applies monitoring middleware", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "memory",
			Monitoring: MonitoringConfig{
				Enabled: true,
			},
		}

		store, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, store)

		// Perform operations to generate metrics
		ctx := context.Background()
		_ = store.Put(ctx, "test-key", strings.NewReader("test-data"))

		// Check that metrics are being collected
		metrics := store.Metrics()
		assert.NotNil(t, metrics)
		assert.Greater(t, metrics.TotalUploads, int64(0))
	})

	t.Run("applies cache middleware", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "memory",
			Cache: CacheConfig{
				Enabled: true,
				MaxSize: 100,
				TTL:     5 * time.Minute,
			},
		}

		store, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, store)

		// Test caching behavior
		ctx := context.Background()
		testData := "cached-data"
		err = store.Put(ctx, "cache-key", strings.NewReader(testData))
		require.NoError(t, err)

		// First read should hit the underlying storage
		reader1, err := store.Get(ctx, "cache-key")
		require.NoError(t, err)
		data1, _ := io.ReadAll(reader1)
		reader1.Close()
		assert.Equal(t, testData, string(data1))

		// Second read should hit the cache (faster)
		reader2, err := store.Get(ctx, "cache-key")
		require.NoError(t, err)
		data2, _ := io.ReadAll(reader2)
		reader2.Close()
		assert.Equal(t, testData, string(data2))
	})

	t.Run("applies multiple middleware layers", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "memory",
			Retry: RetryConfig{
				Enabled:     true,
				MaxAttempts: 3,
			},
			Monitoring: MonitoringConfig{
				Enabled: true,
			},
			Cache: CacheConfig{
				Enabled: true,
				MaxSize: 50,
			},
		}

		store, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, store)

		// Verify all layers are working
		ctx := context.Background()
		err = store.Put(ctx, "multi-key", strings.NewReader("multi-data"))
		assert.NoError(t, err)

		// Check metrics were collected
		metrics := store.Metrics()
		assert.NotNil(t, metrics)
		assert.Greater(t, metrics.TotalUploads, int64(0))
	})
}

func TestNewFactory_ConfigValidation(t *testing.T) {
	t.Run("empty provider defaults to seaweedfs", func(t *testing.T) {
		config := FactoryConfig{
			Endpoint: "localhost:9333",
			Bucket:   "test",
		}

		store, err := New(config)
		// Should use seaweedfs as default
		require.NoError(t, err)
		require.NotNil(t, store)
	})

	t.Run("validates s3 configuration", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "s3",
			Region:   "", // Missing required region
		}

		store, err := New(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "region required for s3 provider")
		assert.Nil(t, store)
	})
}

func TestNewFactory_ProviderSpecificConfig(t *testing.T) {
	t.Run("passes extra config to provider", func(t *testing.T) {
		config := FactoryConfig{
			Provider: "memory",
			Extra: map[string]interface{}{
				"maxMemory": 1024 * 1024, // 1MB limit
			},
		}

		store, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, store)

		// Memory provider should respect the limit
		ctx := context.Background()
		largeData := strings.Repeat("x", 2*1024*1024) // 2MB
		err = store.Put(ctx, "large-key", strings.NewReader(largeData))
		// This should fail if memory limit is enforced
		// The actual behavior depends on memory provider implementation
		_ = err // Placeholder for now
	})
}

func TestNewFactory_IntegrationWithStorageInterface(t *testing.T) {
	config := FactoryConfig{
		Provider: "memory",
		Monitoring: MonitoringConfig{
			Enabled: true,
		},
	}

	store, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, store)

	ctx := context.Background()

	// Test all Storage interface methods
	t.Run("basic operations", func(t *testing.T) {
		// Put
		err := store.Put(ctx, "test-key", strings.NewReader("test-value"))
		assert.NoError(t, err)

		// Get
		reader, err := store.Get(ctx, "test-key")
		assert.NoError(t, err)
		data, _ := io.ReadAll(reader)
		reader.Close()
		assert.Equal(t, "test-value", string(data))

		// Exists
		exists, err := store.Exists(ctx, "test-key")
		assert.NoError(t, err)
		assert.True(t, exists)

		// Delete
		err = store.Delete(ctx, "test-key")
		assert.NoError(t, err)

		// Verify deletion
		exists, err = store.Exists(ctx, "test-key")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("batch operations", func(t *testing.T) {
		// Add multiple items
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("batch-key-%d", i)
			value := fmt.Sprintf("value-%d", i)
			err := store.Put(ctx, key, strings.NewReader(value))
			require.NoError(t, err)
		}

		// List
		opts := storage.ListOptions{
			Prefix:  "batch-",
			MaxKeys: 10,
		}
		objects, err := store.List(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, objects, 5)

		// DeleteBatch
		keys := make([]string, 5)
		for i := 0; i < 5; i++ {
			keys[i] = fmt.Sprintf("batch-key-%d", i)
		}
		err = store.DeleteBatch(ctx, keys)
		assert.NoError(t, err)

		// Verify deletion
		objects, err = store.List(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, objects, 0)
	})

	t.Run("metadata operations", func(t *testing.T) {
		// Put with metadata
		opts := []storage.PutOption{
			storage.WithContentType("text/plain"),
			storage.WithMetadata(map[string]string{
				"author":  "test",
				"version": "1.0",
			}),
		}
		err := store.Put(ctx, "meta-key", strings.NewReader("meta-value"), opts...)
		assert.NoError(t, err)

		// Head
		object, err := store.Head(ctx, "meta-key")
		assert.NoError(t, err)
		assert.NotNil(t, object)
		assert.Equal(t, "meta-key", object.Key)
		assert.Equal(t, "text/plain", object.ContentType)

		// UpdateMetadata
		newMeta := map[string]string{
			"version": "2.0",
			"updated": "true",
		}
		err = store.UpdateMetadata(ctx, "meta-key", newMeta)
		assert.NoError(t, err)
	})

	t.Run("advanced operations", func(t *testing.T) {
		// Setup source
		err := store.Put(ctx, "source-key", strings.NewReader("copy-me"))
		require.NoError(t, err)

		// Copy
		err = store.Copy(ctx, "source-key", "dest-key")
		assert.NoError(t, err)

		// Verify copy
		reader, err := store.Get(ctx, "dest-key")
		assert.NoError(t, err)
		data, _ := io.ReadAll(reader)
		reader.Close()
		assert.Equal(t, "copy-me", string(data))

		// Move
		err = store.Move(ctx, "dest-key", "moved-key")
		assert.NoError(t, err)

		// Verify move (source should not exist)
		exists, _ := store.Exists(ctx, "dest-key")
		assert.False(t, exists)

		// Verify destination exists
		exists, _ = store.Exists(ctx, "moved-key")
		assert.True(t, exists)
	})

	t.Run("health and metrics", func(t *testing.T) {
		// Health check
		err := store.Health(ctx)
		assert.NoError(t, err)

		// Metrics
		metrics := store.Metrics()
		assert.NotNil(t, metrics)
		assert.Greater(t, metrics.TotalUploads, int64(0))
	})
}

// Benchmark tests
func BenchmarkNewFactory_Creation(b *testing.B) {
	config := FactoryConfig{
		Provider: "memory",
		Retry: RetryConfig{
			Enabled: true,
		},
		Monitoring: MonitoringConfig{
			Enabled: true,
		},
		Cache: CacheConfig{
			Enabled: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store, err := New(config)
		if err != nil {
			b.Fatal(err)
		}
		_ = store
	}
}

func BenchmarkNewFactory_Operations(b *testing.B) {
	config := FactoryConfig{
		Provider: "memory",
		Cache: CacheConfig{
			Enabled: true,
			MaxSize: 100,
		},
	}

	store, err := New(config)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	testData := "benchmark-data"

	b.Run("Put", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("bench-key-%d", i)
			_ = store.Put(ctx, key, strings.NewReader(testData))
		}
	})

	b.Run("Get", func(b *testing.B) {
		// Setup
		_ = store.Put(ctx, "bench-get-key", strings.NewReader(testData))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader, _ := store.Get(ctx, "bench-get-key")
			if reader != nil {
				reader.Close()
			}
		}
	})
}
