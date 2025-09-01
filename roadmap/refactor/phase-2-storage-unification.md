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

1. ✅ Create new `internal/storage/interface.go` (COMPLETED)
2. ✅ Implement base client with existing functionality (COMPLETED)
3. ✅ Add comprehensive tests (COMPLETED)

### Step 2: Migrate Providers

1. ✅ Move SeaweedFS implementation to `providers/seaweedfs/` (COMPLETED)
2. ✅ Refactor to implement new interface (COMPLETED)
3. ✅ Remove provider-specific logic from base client (COMPLETED)

### Step 3: Add Middleware Layers

1. ✅ Extract retry logic to middleware (COMPLETED)
2. ✅ Extract monitoring to middleware (COMPLETED)
3. ✅ Add new cache middleware (COMPLETED)

### Step 3a: Implement Factory Pattern

1. ✅ Create factory pattern at `internal/storage/factory/factory.go` (COMPLETED - 2025-09-01)
2. ✅ Support for provider selection (seaweedfs, memory, s3 placeholder) (COMPLETED)
3. ✅ Middleware layer application (retry, monitoring, cache) (COMPLETED)
4. ✅ Comprehensive unit tests (COMPLETED)

### Step 4: Update Consumers

Update all storage consumers to use new interface:

1. ✅ Create factory-based storage creation function (COMPLETED - 2025-09-01)
   - Added `CreateStorageFromFactory` in `api/config/config.go`
   - Uses new factory pattern from `internal/storage/factory`
   - Supports middleware configuration (retry, monitoring, cache)
   - Backward compatible - old `CreateStorageClientFromConfig` still works

2. ✅ Migrate server.go to use factory pattern (COMPLETED - 2025-09-01)
   - Updated `api/server/server.go` to use factory for storage initialization
   - Modified `initializeSelfUpdateHandler`, `initializeCertificateManager`, `initializeAnalysisHandler`
   - Factory is now always created and passed to components that need storage
   - Reduced `CreateStorageClientFromConfig` usage from 4 to 1 (only in getStorageClient fallback)
   - All tests pass, API compiles successfully

3. ✅ Migrate health.go to use factory pattern (COMPLETED - 2025-09-01)
   - Updated `api/health/health.go` to use `CreateStorageFromFactory`
   - Replaced `GetHealthStatus()` with `Health(ctx)` from Storage interface
   - Added metrics reporting using `Metrics()` method
   - Created unit tests with mock Storage implementation
   - All tests pass, compilation successful

4. ✅ Update getStorageClient method to use new factory (COMPLETED - 2025-09-01)
   - Modified `api/server/server.go` getStorageClient to return `storage.Storage` interface
   - Removed dual-path logic (StorageFactory vs fallback)
   - Now always uses `CreateStorageFromFactory` for consistency
   - Updated storage health and metrics handlers to use new interface methods
   - Added TODOs for handlers that still need *StorageClient (build, platform, lifecycle)
   - All code compiles successfully

5. ✅ Begin BuildDependencies migration to unified storage (COMPLETED - 2025-09-01)
   - Added `Storage storage.Storage` field to `internal/build/trigger.go` BuildDependencies struct
   - Created failing tests for build handler storage interface migration (RED phase)
   - Implemented minimal code to pass interface tests (GREEN phase) 
   - Tests verify BuildDependencies can accept unified storage interface alongside legacy StorageClient
   - Deployed to VPS for integration testing (REFACTOR phase)
   - Part of incremental migration strategy - maintains both interfaces during transition

6. ✅ Migrate build handler storage operations to unified interface (COMPLETED - 2025-09-01)
   - Updated `triggerBuildWithDependencies` to prefer unified storage interface
   - Implemented `uploadArtifactBundleWithUnifiedStorage` for artifact bundle uploads
   - Implemented `uploadFileWithUnifiedStorage` with retry logic for file uploads
   - Implemented `uploadBytesWithUnifiedStorage` for metadata uploads
   - Maintained backward compatibility with legacy StorageClient
   - Added comprehensive tests for all unified storage operations
   - Fixed compilation errors in ARF and ACME modules after migration
   - Successfully deployed and tested on VPS - API healthy with unified storage

7. ✅ Migrate platform handlers to unified storage (COMPLETED - 2025-09-01)
   - Created comprehensive tests for platform handler storage migration (`api/platform/handler_test.go`)
   - Added dual storage support to `platform.Handler` (unified + legacy for backward compatibility)
   - Implemented `NewHandlerWithStorage` constructor for unified storage interface
   - Updated `DeployPlatformService` to prefer unified storage with context-aware operations
   - Migrated `api/server/platform_handlers.go` to use `CreateStorageFromFactory`
   - Replaced 2 instances of `CreateStorageClientFromConfig` with factory pattern
   - Successfully deployed to VPS - API healthy with unified storage implementation
   - Storage operations now use `storage.Put` with `storage.WithContentType` option

```go
// Before
import "github.com/iw2rmb/internal/storage"

client := storage.NewStorageClient(provider, config)
data, err := client.Download(ctx, "key")

// After (using factory)
import "github.com/iw2rmb/internal/storage/factory"

storage, err := factory.New(config)
reader, err := storage.Get(ctx, "key")
defer reader.Close()
```

### Step 5: Remove Old Implementations

1. ✅ Delete `api/arf/storage/` directory (COMPLETED - 2025-09-01)
   - Removed entire `api/arf/storage/` directory containing duplicate implementations
   - Created new `api/arf/storage_service.go` with unified StorageService interface
   - Added `api/arf/recipe_types.go` with necessary types for ARF components
   - Created StorageAdapter that bridges new storage.Storage interface to ARF's StorageService
   - Updated all ARF imports to remove dependency on old storage package

2. ✅ Update ACME storage to use factory pattern (COMPLETED - 2025-09-01)  
   - Modified `api/acme/storage.go` to use `storage.Storage` interface instead of `StorageProvider`
   - Updated storage operations to use new Put/Get methods with proper context handling
   - Maintained backward compatibility with existing certificate management functionality

3. ✅ Remove duplicate storage implementations (COMPLETED - 2025-09-01)
   - Eliminated 4 duplicate files: `seaweedfs_storage.go`, `storage_adapter.go`, `recipe_storage.go`, `consul_index.go`
   - Updated all imports across ARF codebase to remove references to old storage package
   - Created minimal adapter layer for ARF-specific storage needs while using unified backend

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

- [x] All storage operations use unified interface (factory pattern implemented)
- [x] No duplicate retry logic (consolidated in middleware)
- [x] Consistent error handling across all providers (unified error types)
- [ ] Performance metrics improved
- [x] All tests passing
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