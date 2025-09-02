# Storage Layer Bucket/Key Handling Fix

## Problem Statement

The current storage layer has multiple levels adding bucket prefixes, resulting in malformed paths like:
- `artifacts/artifacts/jobs/123/input.tar` (double bucket)
- `404 Not Found` errors when creating directories

This happens because:
1. ARFService adds bucket prefix: `"artifacts/" + "jobs/123/input.tar"`
2. StorageAdapter has its own bucket and tries to add it again
3. SeaweedFS implementations expect bucket/key separation but receive already-prefixed keys

## Solution Principle

**Bucket should ONLY be initialized at the storage driver level and automatically prepended to ALL paths. No detection logic, no conditional prefixing - just simple, consistent path construction.**

## Current Architecture Issues

### Layer Stack (Top to Bottom):
```
ARFService (bucket: "artifacts")
    ↓ Adds prefix: "artifacts/jobs/123/input.tar"
Storage Interface 
    ↓ Passes full path
StorageAdapter (bucket: "artifacts") 
    ↓ Tries to add bucket again
SeaweedFS Provider
    ↓ Constructs: /artifacts/artifacts/jobs/123/input.tar
SeaweedFS Filer (404 Not Found)
```

## Proposed Architecture

### Clean Layer Stack:
```
ARFService (NO bucket handling)
    ↓ Passes clean key: "jobs/123/input.tar"
Storage Interface 
    ↓ Passes clean key
StorageAdapter (bucket: "artifacts") 
    ↓ ALWAYS prepends: "artifacts/jobs/123/input.tar"
SeaweedFS Provider
    ↓ Uses as full path: /artifacts/jobs/123/input.tar
SeaweedFS Filer (Success)
```

## Implementation Plan

### Phase 1: Remove Bucket Handling from High-Level Services

#### 1.1 Fix ARFService (`api/arf/unified_service.go`)
```go
// CURRENT (WRONG):
func (s *ARFService) Put(ctx context.Context, key string, data []byte) error {
    fullKey := fmt.Sprintf("%s/%s", s.bucket, key)  // REMOVE THIS
    reader := bytes.NewReader(data)
    return s.storage.Put(ctx, fullKey, reader)
}

// FIXED:
func (s *ARFService) Put(ctx context.Context, key string, data []byte) error {
    reader := bytes.NewReader(data)
    return s.storage.Put(ctx, key, reader)  // Pass key as-is
}
```

Apply same fix to:
- `Get(ctx, key)` 
- `Delete(ctx, key)`
- `Exists(ctx, key)`

The ARFService should NOT have a bucket field at all if using StorageAdapter.

### Phase 2: Ensure StorageAdapter Always Prepends Bucket

#### 2.1 Fix StorageAdapter (`internal/storage/adapter.go`)
```go
// Ensure Put ALWAYS prepends bucket, no detection logic
func (a *StorageAdapter) Put(ctx context.Context, key string, reader io.Reader, opts ...PutOption) error {
    // ALWAYS prepend bucket - no conditional logic
    fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
    
    // ... rest of implementation
    _, err := a.provider.PutObject("", fullKey, body, options.ContentType)
    // Note: Pass empty string as bucket since fullKey already has it
    return err
}
```

Apply same pattern to:
- `Get(ctx, key)` 
- `Delete(ctx, key)`
- `Exists(ctx, key)`
- `List(ctx, opts)` - Ensure prefix includes bucket

### Phase 3: Fix SeaweedFS Providers to Handle Full Paths

#### 3.1 Fix SeaweedFS Client (`internal/storage/seaweedfs.go`)
```go
func (c *SeaweedFSClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
    // If bucket is empty, key contains the full path
    var fullPath string
    if bucket == "" {
        fullPath = key
    } else {
        fullPath = fmt.Sprintf("%s/%s", bucket, key)
    }
    
    // Create directory structure
    dir := filepath.Dir(fullPath)
    if dir != "." && dir != "/" {
        if err := c.createDirectory("", dir); err != nil {
            return nil, fmt.Errorf("failed to create directory: %w", err)
        }
    }
    
    // Upload using full path
    url := fmt.Sprintf("%s/%s?replication=%s", c.filerURL, fullPath, c.replication)
    // ... rest of upload logic
}

func (c *SeaweedFSClient) createDirectory(bucket, dir string) error {
    // If bucket is empty, dir contains the full path
    var fullPath string
    if bucket == "" {
        fullPath = dir
    } else {
        fullPath = fmt.Sprintf("%s/%s", bucket, dir)
    }
    
    url := fmt.Sprintf("%s/%s/", c.filerURL, fullPath)
    // ... rest of implementation
}
```

#### 3.2 Fix SeaweedFS Provider (`internal/storage/providers/seaweedfs/client.go`)
Apply the same pattern as 3.1 - when bucket is empty string, treat key as full path.

### Phase 4: Update Storage Initialization

#### 4.1 Server Initialization
When creating storage layers, ensure proper bucket configuration:

```go
// In server initialization
storageProvider := seaweedfs.NewProvider(config)
storageAdapter := storage.NewAdapter(storageProvider, "artifacts")  // Bucket set HERE only

// For ARF service - don't pass bucket if using adapter
arfService := arf.NewARFService(storageAdapter, "")  // Empty bucket since adapter handles it
```

### Phase 5: Testing Plan

#### 5.1 Unit Tests
- Test StorageAdapter always prepends bucket
- Test ARFService passes keys without modification
- Test SeaweedFS handles empty bucket parameter correctly

#### 5.2 Integration Tests
```bash
# Test transformation creates correct paths
curl -X POST "https://api.dev.ployman.app/v1/arf/transforms" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate:java-8-to-11",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/winterbe/java8-tutorial.git",
      "branch": "master"
    }
  }'

# Verify paths in SeaweedFS
curl "http://45.12.75.241:8888/artifacts/jobs/"
# Should see: openrewrite-{timestamp}/input.tar
# NOT: artifacts/jobs/openrewrite-{timestamp}/input.tar
```

#### 5.3 Validation Checklist
- [ ] No double bucket paths in SeaweedFS
- [ ] Transformations complete successfully
- [ ] input.tar uploads correctly
- [ ] output.tar contains all Java source files (not just pom.xml)
- [ ] No 404 errors during directory creation

## Migration Notes

### Breaking Changes
1. ARFService will no longer prepend bucket to keys
2. StorageAdapter will ALWAYS prepend its configured bucket
3. Direct storage users must ensure they're not double-prefixing

### Compatibility
- Existing data paths remain unchanged in SeaweedFS
- Only the path construction logic changes
- No data migration required

## Benefits of This Approach

1. **Simplicity**: No detection logic, no conditionals - bucket always added at driver level
2. **Consistency**: One place handles bucket prefixing (StorageAdapter)
3. **Clarity**: Each layer has a clear responsibility
4. **Maintainability**: Easy to understand and debug path construction
5. **Performance**: No string parsing or path detection overhead

## Implementation Order

1. **Fix StorageAdapter** - Always prepend bucket (no detection)
2. **Fix ARFService** - Remove bucket handling entirely
3. **Fix SeaweedFS implementations** - Handle empty bucket parameter
4. **Update initialization** - Configure bucket only at StorageAdapter level
5. **Test end-to-end** - Verify transformations work with correct paths

## Success Criteria

1. OpenRewrite transformations complete without storage errors
2. Paths in SeaweedFS are correct: `/artifacts/jobs/{id}/input.tar`
3. No double bucket prefixes anywhere
4. output.tar contains complete transformed source code
5. All existing storage operations continue to work

## Rollback Plan

If issues arise:
1. Revert ARFService to add bucket prefix
2. Revert StorageAdapter to previous logic
3. Document any data inconsistencies for manual cleanup

## Future Improvements

Once this fix is stable:
1. Consider removing bucket concept from ARFService entirely
2. Standardize on single SeaweedFS implementation (remove duplicate)
3. Add path validation and logging for debugging
4. Implement storage path conventions documentation