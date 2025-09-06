---
task: 02-kb-learning-pipeline  
parent: h-implement-transflow-mvp
branch: feature/transflow-mvp-completion
status: pending  
created: 2025-01-09
modules: [kb, learning, transflow]
---

# KB Learning Pipeline Implementation

## Problem/Goal
Implement the learning pipeline that reads from healing attempts, writes successful cases to KB, and maintains aggregated summaries. This enables transflow to learn from past healing attempts and improve success rates over time.

## Success Criteria

### RED Phase (Local Unit Tests)
- [ ] Write failing tests for learning recorder interface
- [ ] Write failing tests for case aggregation and summary generation  
- [ ] Write failing tests for duplicate detection and deduplication
- [ ] Write failing tests for confidence scoring algorithms
- [ ] Write failing tests for summary compaction and maintenance
- [ ] All tests fail as expected (learning pipeline doesn't exist yet)

### GREEN Phase (Minimal Implementation)
- [ ] Implement learning recorder in `internal/kb/learning/`
- [ ] Implement case aggregation for summary updates
- [ ] Implement duplicate detection using patch fingerprints
- [ ] Implement confidence scoring based on historical success rates
- [ ] Implement summary compaction (limit top patches, prune old cases)
- [ ] All unit tests pass with coverage >60%
- [ ] `go build ./...` succeeds

### REFACTOR Phase (VPS Integration)  
- [ ] Deploy learning pipeline to VPS
- [ ] Test learning from real healing attempts on VPS
- [ ] Validate concurrent learning with multiple transflow instances
- [ ] Test large-scale aggregation (>10k cases per error type)
- [ ] Performance benchmarks for learning operations (<200ms per case)

## TDD Implementation Plan

### 1. RED: Write Failing Tests First
```go
// Test files to create:
// internal/kb/learning/recorder_test.go
// internal/kb/learning/aggregator_test.go  
// internal/kb/learning/dedup_test.go
// internal/kb/learning/scoring_test.go
// internal/kb/learning/compactor_test.go

func TestLearningRecorder_RecordSuccess(t *testing.T) {
    // Should fail - no recorder exists yet
    recorder := learning.NewRecorder(storage, locker)
    
    healingAttempt := &models.HealingAttempt{
        ErrorSignature: "java-compilation-error-123", 
        Patch: patchContent,
        Success: true,
    }
    
    err := recorder.RecordHealing(ctx, healingAttempt)
    assert.NoError(t, err)
    
    // Verify case was stored in KB
    cases := storage.GetCasesByError(ctx, "java-compilation-error-123")
    assert.Len(t, cases, 1)
}

func TestAggregator_UpdateSummary(t *testing.T) {
    // Should fail - no aggregator exists yet
    aggregator := learning.NewAggregator(storage)
    
    // Add multiple cases for same error
    cases := []*models.Case{successCase1, successCase2, failureCase1}
    
    summary, err := aggregator.UpdateSummary(ctx, "error-sig-123", cases)
    assert.NoError(t, err)
    assert.Equal(t, 0.67, summary.SuccessRate) // 2/3 success rate
}
```

### 2. GREEN: Minimal Implementation
```go
// Files to implement:
// internal/kb/learning/recorder.go - Records healing attempts to KB
// internal/kb/learning/aggregator.go - Aggregates cases into summaries  
// internal/kb/learning/dedup.go - Detects and handles duplicate patches
// internal/kb/learning/scoring.go - Calculates confidence scores
// internal/kb/learning/compactor.go - Maintains summary size limits
```

### 3. REFACTOR: VPS Testing
- Deploy learning components to VPS  
- Run real healing scenarios and validate learning occurs
- Test concurrent learning from multiple transflow instances
- Performance test aggregation with large case volumes

## Learning Pipeline Flow

### 1. Record Healing Attempt
```go
type HealingAttempt struct {
    TransflowID    string    `json:"transflow_id"`
    ErrorSignature string    `json:"error_signature"` // Canonical error ID
    Patch          []byte    `json:"patch"`           // Applied patch content  
    PatchHash      string    `json:"patch_hash"`      // Content fingerprint
    Success        bool      `json:"success"`         // Did healing work?
    BuildLogs      []string  `json:"build_logs"`      // Post-healing build output
    Duration       time.Duration `json:"duration"`    // Time to apply patch
    Timestamp      time.Time `json:"timestamp"`
}

// Learning flow:
// 1. Healing completes (success or failure)
// 2. Recorder.RecordHealing() called with attempt details
// 3. Generate case ID and store in kb/cases/
// 4. Trigger summary update for this error signature
// 5. Aggregator recalculates success rates and top patches
```

### 2. Deduplication Strategy
```go
// Detect duplicate patches by content hash
func (d *Deduplicator) IsDuplicate(patch []byte) (bool, string) {
    hash := d.generatePatchHash(patch)
    existingCase, exists := d.storage.GetCaseByPatchHash(hash)
    return exists, existingCase.ID
}

// Update existing case rather than create duplicate
func (r *Recorder) RecordHealing(attempt *HealingAttempt) error {
    isDup, existingID := r.dedup.IsDuplicate(attempt.Patch)
    if isDup {
        return r.updateExistingCase(existingID, attempt)
    }
    return r.createNewCase(attempt)
}
```

### 3. Confidence Scoring
```go
// Calculate confidence based on historical success
func (s *Scorer) CalculateConfidence(errorSig string, patch []byte) float64 {
    summary := s.storage.GetSummary(errorSig)
    if summary == nil {
        return 0.5 // Default confidence for unknown patterns
    }
    
    // Find similar patches in history
    similarPatches := s.findSimilarPatches(patch, summary.TopPatches)
    if len(similarPatches) == 0 {
        return summary.SuccessRate // Use overall success rate
    }
    
    // Weight by similarity and historical success
    return s.weightedConfidence(similarPatches)
}
```

### 4. Summary Compaction  
```go
// Maintain manageable summary sizes
func (c *Compactor) CompactSummary(summary *models.Summary) error {
    // Keep only top 10 most successful patches
    if len(summary.TopPatches) > 10 {
        sort.Slice(summary.TopPatches, func(i, j int) bool {
            return summary.TopPatches[i].SuccessRate > summary.TopPatches[j].SuccessRate
        })
        summary.TopPatches = summary.TopPatches[:10]
    }
    
    // Archive old cases (keep only last 100 per error type)
    return c.archiveOldCases(summary.ErrorID, 100)
}
```

## Context Files
- @internal/transflow/healing/ - Healing workflow integration points
- @internal/transflow/runner.go - Where learning recording should be triggered
- @roadmap/transflow/kb.md - KB learning requirements and specifications

## User Notes

**Learning Triggers:**
- Record after every healing attempt (success or failure)  
- Batch summary updates (max 1 update per error per minute)
- Run compaction during off-peak hours (configurable schedule)

**Concurrency Handling:**
- Use Consul locks for summary updates across multiple transflow instances
- Implement retry logic with exponential backoff for lock contention
- Graceful degradation if KB learning fails (don't block healing workflow)

**Performance Requirements:**
- Learning recording: <200ms per healing attempt
- Summary updates: <1s for aggregation of 100 cases
- Compaction: <10s for maintenance of 10k cases
- Memory usage: <100MB for in-memory caches

**Error Handling:**
- KB learning failures should not block transflow execution
- Log learning errors but continue with healing workflow  
- Implement circuit breaker for persistent KB failures
- Fallback to basic healing without learning if KB unavailable

## Work Log  
- [2025-01-09] Created KB learning pipeline subtask with comprehensive flow design