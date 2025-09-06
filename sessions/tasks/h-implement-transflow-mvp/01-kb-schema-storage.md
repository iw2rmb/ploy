---
task: 01-kb-schema-storage
parent: h-implement-transflow-mvp
branch: feature/transflow-mvp-completion  
status: pending
created: 2025-01-09
modules: [kb, storage, models]
---

# KB Schema & Storage Implementation

## Problem/Goal
Design and implement the Knowledge Base data structures, storage layer, and basic CRUD operations following SeaweedFS patterns. The KB stores error cases, successful patches, and learning summaries to improve transflow healing success rates.

## Success Criteria

### RED Phase (Local Unit Tests)
- [ ] Write failing tests for KB models (Error, Case, Summary, Patch)
- [ ] Write failing tests for KB storage interface (Store, Retrieve, List, Delete)
- [ ] Write failing tests for error signature canonicalization
- [ ] Write failing tests for patch fingerprint generation
- [ ] All tests fail as expected (models/interfaces don't exist yet)

### GREEN Phase (Minimal Implementation)  
- [ ] Define KB data models in `internal/kb/models/`
- [ ] Implement KB storage layer in `internal/kb/storage/`
- [ ] Implement error signature generation (normalized, deduplicated)
- [ ] Implement patch fingerprinting (content hash, structural analysis)
- [ ] All unit tests pass with coverage >60%
- [ ] `go build ./...` succeeds

### REFACTOR Phase (VPS Integration)
- [ ] Deploy KB storage to VPS SeaweedFS under `kb/` namespace  
- [ ] Run integration tests on VPS with real SeaweedFS
- [ ] Validate concurrent access and locking via Consul KV
- [ ] Test large dataset operations (>1000 cases)
- [ ] Performance benchmarks established (store <100ms, retrieve <50ms)

## TDD Implementation Plan

### 1. RED: Write Failing Tests First
```go
// Test files to create:
// internal/kb/models/error_test.go
// internal/kb/models/case_test.go  
// internal/kb/models/summary_test.go
// internal/kb/storage/kb_storage_test.go
// internal/kb/fingerprint/patch_test.go

func TestErrorSignatureGeneration(t *testing.T) {
    // Should fail - no Error model exists yet
    error := models.NewError("compilation failed", "Main.java:15", buildLogs)
    signature := error.GenerateSignature() 
    assert.NotEmpty(t, signature)
}

func TestKBStorage_StoreCaseRetrieve(t *testing.T) {
    // Should fail - no KB storage exists yet  
    storage := NewKBStorage(config)
    case := &models.Case{ID: "test", Error: errorData, Patch: patchData}
    err := storage.StoreCase(ctx, case)
    assert.NoError(t, err)
}
```

### 2. GREEN: Minimal Implementation
```go  
// Files to implement:
// internal/kb/models/error.go - Error case data model
// internal/kb/models/case.go - Learning case with error+patch  
// internal/kb/models/summary.go - Aggregated learning summaries
// internal/kb/storage/kb_storage.go - SeaweedFS KB operations
// internal/kb/fingerprint/patch.go - Patch content fingerprinting
```

### 3. REFACTOR: VPS Testing  
- Deploy to VPS and test with real SeaweedFS cluster
- Validate Consul KV locking for concurrent operations
- Performance test with realistic data volumes

## Context Files
- @internal/storage/recipe_storage.go - Existing SeaweedFS patterns
- @internal/storage/artifacts.go - Storage interface patterns  
- @roadmap/transflow/kb.md - KB specification and requirements
- @api/arf/models/ - Existing model patterns to follow

## User Notes

**KB Data Model Requirements:**
```go
type Error struct {
    ID        string    `json:"id"`        // Generated from signature
    Signature string    `json:"signature"` // Canonical error pattern  
    Message   string    `json:"message"`   // Original error message
    Location  string    `json:"location"`  // File:line where error occurred
    BuildLogs []string  `json:"build_logs"` // Relevant log excerpts
    Created   time.Time `json:"created"`
}

type Case struct {
    ID         string    `json:"id"`         // UUID for this learning case
    ErrorID    string    `json:"error_id"`   // Links to Error via signature
    PatchHash  string    `json:"patch_hash"` // Fingerprint of successful patch
    Patch      []byte    `json:"patch"`      // Actual patch content (diff)
    Success    bool      `json:"success"`    // Whether this patch worked
    Confidence float64   `json:"confidence"` // Success probability (0.0-1.0)
    Created    time.Time `json:"created"`
}

type Summary struct {
    ErrorID     string             `json:"error_id"`    // Links to Error signature
    CaseCount   int               `json:"case_count"`   // Number of cases seen
    SuccessRate float64           `json:"success_rate"` // Overall success rate
    TopPatches  []PatchSummary    `json:"top_patches"`  // Most successful patches
    Updated     time.Time         `json:"updated"`
}
```

**Storage Layout:**
- `kb/errors/{signature}` - Error definitions by canonical signature
- `kb/cases/{case_id}` - Individual learning cases  
- `kb/summaries/{error_signature}` - Aggregated summaries per error type
- `kb/patches/{hash}` - Deduplicated patch content

**Locking Strategy:**
- Use Consul KV for distributed locking during concurrent updates
- Lock keys: `kb/locks/summary/{error_signature}` for summary updates
- Timeout: 30s max lock duration with automatic cleanup

## Work Log
- [2025-01-09] Created KB schema subtask with comprehensive TDD plan