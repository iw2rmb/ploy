# Phase 2: Storage Layer Unification

## Objective

Create a single, unified storage interface with proper abstraction layers, eliminating redundant implementations and establishing a consistent storage pattern across all components.

## Current State Analysis

### Redundant Implementations

1. **Multiple Storage Clients**:
   - `internal/storage/client.go` - Main implementation
   - `internal/storage/seaweedfs.go` - SeaweedFS specific
   - `api/arf/storage/seaweedfs_storage.go` - ARF-specific SeaweedFS
   - `api/arf/storage/storage_adapter.go` - ARF adapter pattern
   - `api/acme/storage.go` - ACME-specific storage

2. **Duplicate Retry Logic**:
   - `internal/storage/retry.go` - Generic retry
   - Inline retry logic in multiple handlers
   - Custom retry in ARF dispatcher

3. **Scattered Error Handling**:
   - `internal/storage/errors.go`
   - Custom error types in each module
   - Inconsistent error wrapping

## Proposed Architecture

```
internal/storage/
├── README.md                    # Storage package documentation
├── interface.go                 # Core storage interface
├── client.go                    # Base client implementation
├── providers/
│   ├── seaweedfs/
│   │   ├── client.go           # SeaweedFS implementation
│   │   ├── config.go           # SeaweedFS configuration
│   │   └── client_test.go      # SeaweedFS tests
│   ├── s3/
│   │   ├── client.go           # S3 implementation
│   │   └── config.go           # S3 configuration
│   └── memory/
│       └── client.go           # In-memory implementation for testing
├── middleware/
│   ├── retry.go                # Retry middleware
│   ├── monitoring.go           # Metrics and monitoring
│   ├── logging.go              # Logging middleware
│   ├── encryption.go           # Encryption layer
│   └── cache.go                # Caching layer
├── errors.go                   # Unified error types
└── factory.go                  # Storage factory pattern
```

## Core Interface Design

```go
// internal/storage/interface.go
package storage

import (
    "context"
    "io"
    "time"
)

// Object represents a stored object with metadata
type Object struct {
    Key          string
    Size         int64
    ContentType  string
    ETag         string
    LastModified time.Time
    Metadata     map[string]string
}

// ListOptions configures object listing
type ListOptions struct {
    Prefix      string
    MaxKeys     int
    Delimiter   string
    StartAfter  string
}

// Storage defines the core storage interface
type Storage interface {
    // Basic operations
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    
    // Batch operations
    List(ctx context.Context, opts ListOptions) ([]Object, error)
    DeleteBatch(ctx context.Context, keys []string) error
    
    // Metadata operations
    Head(ctx context.Context, key string) (*Object, error)
    UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error
    
    // Advanced operations
    Copy(ctx context.Context, src, dst string) error
    Move(ctx context.Context, src, dst string) error
    
    // Health and metrics
    Health(ctx context.Context) error
    Metrics() StorageMetrics
}

// PutOption configures Put operations
type PutOption func(*putOptions)

type putOptions struct {
    ContentType string
    Metadata    map[string]string
    CacheControl string
}

func WithContentType(ct string) PutOption
func WithMetadata(m map[string]string) PutOption
func WithCacheControl(cc string) PutOption
```

## Middleware Pattern

```go
// internal/storage/middleware/retry.go
package middleware

import (
    "context"
    "time"
    "github.com/iw2rmb/internal/storage"
)

type RetryMiddleware struct {
    next          storage.Storage
    maxAttempts   int
    backoff       time.Duration
    maxBackoff    time.Duration
}

func NewRetryMiddleware(next storage.Storage, opts ...RetryOption) *RetryMiddleware {
    rm := &RetryMiddleware{
        next:        next,
        maxAttempts: 3,
        backoff:     100 * time.Millisecond,
        maxBackoff:  30 * time.Second,
    }
    for _, opt := range opts {
        opt(rm)
    }
    return rm
}

func (r *RetryMiddleware) Get(ctx context.Context, key string) (io.ReadCloser, error) {
    var lastErr error
    for attempt := 0; attempt < r.maxAttempts; attempt++ {
        reader, err := r.next.Get(ctx, key)
        if err == nil {
            return reader, nil
        }
        if !isRetryable(err) {
            return nil, err
        }
        lastErr = err
        if attempt < r.maxAttempts-1 {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(r.calculateBackoff(attempt)):
            }
        }
    }
    return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

## Factory Pattern

```go
// internal/storage/factory.go
package storage

import (
    "fmt"
    "github.com/iw2rmb/internal/storage/providers/seaweedfs"
    "github.com/iw2rmb/internal/storage/providers/s3"
    "github.com/iw2rmb/internal/storage/middleware"
)

type Config struct {
    Provider   string                 // "seaweedfs", "s3", "memory"
    Endpoint   string
    Bucket     string
    Region     string
    Retry      RetryConfig
    Monitoring MonitoringConfig
    Cache      CacheConfig
    Extra      map[string]interface{} // Provider-specific config
}

func New(cfg Config) (Storage, error) {
    // Create base provider
    var base Storage
    switch cfg.Provider {
    case "seaweedfs":
        base = seaweedfs.New(cfg)
    case "s3":
        base = s3.New(cfg)
    case "memory":
        base = memory.New()
    default:
        return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
    }
    
    // Apply middleware layers
    storage := base
    
    // Add retry layer
    if cfg.Retry.Enabled {
        storage = middleware.NewRetryMiddleware(storage, cfg.Retry)
    }
    
    // Add monitoring layer
    if cfg.Monitoring.Enabled {
        storage = middleware.NewMonitoringMiddleware(storage, cfg.Monitoring)
    }
    
    // Add cache layer
    if cfg.Cache.Enabled {
        storage = middleware.NewCacheMiddleware(storage, cfg.Cache)
    }
    
    return storage, nil
}
```

## Migration Strategy

### Step 1: Implement Core Interface

1. Create new `internal/storage/interface.go`
2. Implement base client with existing functionality
3. Add comprehensive tests

### Step 2: Migrate Providers

1. Move SeaweedFS implementation to `providers/seaweedfs/`
2. Refactor to implement new interface
3. Remove provider-specific logic from base client

### Step 3: Add Middleware Layers

1. ✅ Extract retry logic to middleware (COMPLETED)
2. ✅ Extract monitoring to middleware (COMPLETED)
3. ✅ Add new cache middleware (COMPLETED)

### Step 4: Update Consumers

Update all storage consumers to use new interface:

```go
// Before
import "github.com/iw2rmb/internal/storage"

client := storage.NewStorageClient(provider, config)
data, err := client.Download(ctx, "key")

// After
import "github.com/iw2rmb/internal/storage"

client, err := storage.New(config)
reader, err := client.Get(ctx, "key")
defer reader.Close()
```

### Step 5: Remove Old Implementations

1. Delete `api/arf/storage/` directory
2. Delete duplicate storage implementations
3. Update all imports

## Error Handling Unification

```go
// internal/storage/errors.go
package storage

import "fmt"

type ErrorCode string

const (
    ErrNotFound      ErrorCode = "NOT_FOUND"
    ErrAccessDenied  ErrorCode = "ACCESS_DENIED"
    ErrAlreadyExists ErrorCode = "ALREADY_EXISTS"
    ErrInvalidKey    ErrorCode = "INVALID_KEY"
    ErrTimeout       ErrorCode = "TIMEOUT"
    ErrInternal      ErrorCode = "INTERNAL"
)

type StorageError struct {
    Code    ErrorCode
    Message string
    Key     string
    Cause   error
}

func (e *StorageError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (key=%s): %v", e.Code, e.Message, e.Key, e.Cause)
    }
    return fmt.Sprintf("%s: %s (key=%s)", e.Code, e.Message, e.Key)
}

func IsNotFound(err error) bool
func IsAccessDenied(err error) bool
func IsAlreadyExists(err error) bool
```

## Performance Optimizations

### Connection Pooling
```go
type PoolConfig struct {
    MaxConnections int
    MaxIdle        int
    IdleTimeout    time.Duration
}
```

### Batch Operations
```go
// Optimize batch operations with goroutines
func (c *Client) DeleteBatch(ctx context.Context, keys []string) error {
    const batchSize = 100
    errors := make(chan error, len(keys))
    
    for i := 0; i < len(keys); i += batchSize {
        batch := keys[i:min(i+batchSize, len(keys))]
        go func(b []string) {
            for _, key := range b {
                if err := c.Delete(ctx, key); err != nil {
                    errors <- err
                    return
                }
            }
            errors <- nil
        }(batch)
    }
    
    // Collect results...
}
```

## Testing Strategy

### Unit Tests
```go
// internal/storage/providers/memory/client_test.go
func TestMemoryStorage(t *testing.T) {
    storage := memory.New()
    testutil.RunStorageTestSuite(t, storage)
}
```

### Integration Tests
```go
// internal/storage/integration_test.go
func TestSeaweedFSIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    storage, err := New(Config{
        Provider: "seaweedfs",
        Endpoint: os.Getenv("SEAWEEDFS_ENDPOINT"),
    })
    require.NoError(t, err)
    
    testutil.RunStorageTestSuite(t, storage)
}
```

## Metrics & Monitoring

```go
type StorageMetrics struct {
    Operations map[string]*OperationMetrics
    Errors     map[ErrorCode]int64
    BytesIn    int64
    BytesOut   int64
}

type OperationMetrics struct {
    Count       int64
    Duration    time.Duration
    Errors      int64
    LastError   error
    LastSuccess time.Time
}
```

## Validation Checklist

- [ ] All storage operations use unified interface
- [ ] No duplicate retry logic
- [ ] Consistent error handling across all providers
- [ ] Performance metrics improved
- [ ] All tests passing
- [ ] Documentation updated

## Migration Steps

- Create new interface and base implementation
- Migrate SeaweedFS provider
- Implement middleware layers
- Update consumer code
- Testing and validation
- Remove old implementations
- Documentation and cleanup

## Expected Outcomes

### Before
- Storage implementations: 5+
- Duplicate retry logic: 3 locations
- Storage-related LOC: ~8,000

### After
- Storage implementations: 1 interface, 3 providers
- Retry logic: 1 middleware
- Storage-related LOC: ~5,000 (37% reduction)
- Performance: 15% improvement in storage operations
- Maintainability: Single point of modification for storage logic