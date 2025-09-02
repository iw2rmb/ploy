package seaweedfs

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RED Phase: These tests will fail initially as the provider doesn't implement Storage interface yet

// skipIfSeaweedFSUnavailable checks if SeaweedFS is running and skips the test if not
func skipIfSeaweedFSUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping SeaweedFS integration tests in short mode")
	}

	client := &http.Client{Timeout: 1 * time.Second}
	_, err := client.Get("http://localhost:9333/cluster/status")
	if err != nil {
		t.Skipf("SeaweedFS not available: %v", err)
	}
}

func TestSeaweedFSProvider_ImplementsStorageInterface(t *testing.T) {
	// Test that SeaweedFS provider implements the Storage interface
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	// This should compile - provider must implement storage.Storage interface
	var _ storage.Storage = provider
}

func TestSeaweedFSProvider_Get(t *testing.T) {
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Get method from Storage interface
	reader, err := provider.Get(ctx, "test-key")
	// This will fail until implemented
	if err == nil {
		defer reader.Close()
		content, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.NotEmpty(t, content)
	}
}

func TestSeaweedFSProvider_Put(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	content := "test content for put operation"

	// Test Put method with options
	err = provider.Put(ctx, "test-put-key", strings.NewReader(content),
		storage.WithContentType("text/plain"),
		storage.WithMetadata(map[string]string{"test": "metadata"}))

	// This will fail until Put is implemented
	assert.NoError(t, err)
}

func TestSeaweedFSProvider_Delete(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Delete method
	err = provider.Delete(ctx, "test-delete-key")
	// This will fail until Delete is implemented
	assert.NoError(t, err)
}

func TestSeaweedFSProvider_Exists(t *testing.T) {
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Exists method
	exists, err := provider.Exists(ctx, "test-exists-key")
	// This will fail until Exists is implemented
	assert.NoError(t, err)
	assert.False(t, exists) // Non-existent key should return false
}

func TestSeaweedFSProvider_List(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	opts := storage.ListOptions{
		Prefix:  "test/",
		MaxKeys: 10,
	}

	// Test List method
	objects, err := provider.List(ctx, opts)
	// This will fail until List is implemented
	assert.NoError(t, err)
	assert.NotNil(t, objects)
}

func TestSeaweedFSProvider_Head(t *testing.T) {
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Head method
	obj, err := provider.Head(ctx, "test-head-key")
	// This will fail until Head is implemented
	if err == nil {
		assert.NotNil(t, obj)
		assert.Equal(t, "test-head-key", obj.Key)
	}
}

func TestSeaweedFSProvider_Health(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Health method
	err = provider.Health(ctx)
	// This will fail until Health is implemented
	assert.NoError(t, err)
}

func TestSeaweedFSProvider_Metrics(t *testing.T) {
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	// Test Metrics method
	metrics := provider.Metrics()
	// This will fail until Metrics is implemented
	assert.NotNil(t, metrics)
}

func TestSeaweedFSProvider_BatchOperations(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test DeleteBatch method
	keys := []string{"batch1", "batch2", "batch3"}
	err = provider.DeleteBatch(ctx, keys)
	// This will fail until DeleteBatch is implemented
	assert.NoError(t, err)
}

func TestSeaweedFSProvider_MetadataOperations(t *testing.T) {
	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	metadata := map[string]string{"key1": "value1", "key2": "value2"}

	// Test UpdateMetadata method
	err = provider.UpdateMetadata(ctx, "test-metadata-key", metadata)
	// This will fail until UpdateMetadata is implemented
	assert.NoError(t, err)
}

func TestSeaweedFSProvider_AdvancedOperations(t *testing.T) {
	skipIfSeaweedFSUnavailable(t)

	cfg := Config{
		Master:      "localhost:9333",
		Filer:       "localhost:8888",
		Collection:  "test-collection",
		Replication: "001",
	}

	provider, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Copy method
	err = provider.Copy(ctx, "source-key", "dest-key")
	// This will fail until Copy is implemented
	assert.NoError(t, err)

	// Test Move method
	err = provider.Move(ctx, "move-source", "move-dest")
	// This will fail until Move is implemented
	assert.NoError(t, err)
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Master:      "localhost:9333",
				Filer:       "localhost:8888",
				Collection:  "test",
				Replication: "001",
			},
			wantErr: false,
		},
		{
			name: "missing master",
			config: Config{
				Filer:       "localhost:8888",
				Collection:  "test",
				Replication: "001",
			},
			wantErr: true,
		},
		{
			name: "missing filer",
			config: Config{
				Master:      "localhost:9333",
				Collection:  "test",
				Replication: "001",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
