---
task: m-implement-error-patch-deduplication
branch: feature/implement-error-patch-deduplication
status: pending
created: 2025-09-05
modules: [internal/cli/transflow, internal/storage, internal/orchestration]
---

# Error/Patch Deduplication

## Problem/Goal
Enhance the transflow KB persistence layer with advanced deduplication capabilities to minimize storage usage and improve query performance. While basic content-addressed storage exists for patches, we need sophisticated deduplication for error signatures and intelligent patch similarity detection to prevent storage bloat and enable faster KB lookups.

## Success Criteria
- [ ] Implement fuzzy error signature matching to detect similar errors across different environments
- [ ] Add patch similarity detection using diff algorithms to group related fixes
- [ ] Create storage compaction system to merge duplicate/similar cases automatically
- [ ] Implement KB maintenance jobs that periodically deduplicate historical data
- [ ] Add metrics and monitoring for deduplication effectiveness
- [ ] Ensure backward compatibility with existing KB data structure
- [ ] Performance improvement: 50%+ reduction in duplicate storage usage
- [ ] Performance improvement: 25%+ faster KB summary generation through reduced data

## Context Manifest

### How KB Storage Currently Works: Transflow Knowledge Base Persistence

When a transflow healing workflow completes, the system currently captures and stores healing cases through a comprehensive KB persistence layer that was recently implemented. The process begins when the `KBIntegration.WriteHealingCase()` method is called with the healing context, attempt details, outcome, and build logs.

The KB storage architecture uses a **hierarchical content-addressed storage pattern** built on top of SeaweedFS and Consul. Error signatures are generated using `DefaultSignatureGenerator.GenerateSignature()` which normalizes error output by removing environment-specific details (timestamps via `timestampPattern`, file paths via `pathPattern`, line numbers via `lineNumberPattern`, memory addresses, temp files, user home directories, build IDs, and thread IDs). The normalization process extracts key error patterns using common error indicators across languages, then creates a SHA-256 hash of the normalized content combined with language and compiler information, truncated to 8 bytes (16 hex characters) for compact storage keys.

**Storage Key Hierarchy:**
- Cases: `kb/healing/errors/{lang}/{signature}/cases/{run_id}.json`
- Summaries: `kb/healing/errors/{lang}/{signature}/summary.json`  
- Patches: `kb/healing/patches/{patch_fingerprint}.patch`
- Snapshots: `kb/healing/snapshot.json`
- Locks: `kb/locks/{lang}/{signature}` (Consul KV)

The `SeaweedFSKBStorage` implementation handles all storage operations through the unified storage interface, using JSON serialization for structured data. Each case record contains complete context including language, lane, repo URL, dependency hashes, compiler version, build command, source files, and metadata. The healing attempt captures the type (orw_recipe, llm_patch, human_step), recipe name or patch fingerprint, and any additional instructions. The outcome records success status, build result, duration, and completion timestamp.

**Patch Storage and Fingerprinting:**
Patches undergo normalization through `DefaultSignatureGenerator.NormalizePatch()` which removes diff headers with timestamps, git-specific index lines, and diff command lines while preserving actual content. File references are normalized to `[FILE_A]` and `[FILE_B]` placeholders. The normalized patch content is SHA-256 hashed to create a fingerprint for content-addressed storage. This basic deduplication prevents storing identical patches multiple times, but doesn't detect semantically similar patches with different formatting.

**Summary Computation and Promotion:**
The `SummaryComputer` analyzes cases to identify fix candidates and promote successful approaches. It examines all cases for a given error signature, tracking wins/failures for each recipe or patch fingerprint. The scoring algorithm uses weighted factors: success rate (30%), frequency of use (40%), and recency of last success (30%). Fixes are promoted to the summary after meeting minimum thresholds (≥3 cases, ≥60% success rate, ≥0.5 composite score). The system maintains up to 10 promoted fixes per error signature, ranked by score.

**Distributed Locking and Concurrency:**
Summary updates use Consul-based distributed locking via `ConsulKBLockManager` to prevent race conditions. Lock keys follow the pattern `kb/locks/{lang}/{signature}` with configurable TTL (default 5 seconds). The `TryWithLockRetry` mechanism attempts acquisition up to 3 times with 100ms intervals. If lock acquisition fails during case writing, the system continues without summary updates, relying on periodic compactor jobs to rebuild summaries later.

**Integration with Transflow Workflow:**
The KB integrates with healing workflows through `KBIntegration.LoadKBContext()` which retrieves recommended fixes for error signatures. If confidence scores exceed the read threshold (default 70%), the system converts KB fixes to branch specifications and bypasses LLM planning. Otherwise, KB context informs but doesn't replace planning. The `ExtendedJobSubmissionHelper` wraps the standard job submitter to enable KB-aware planning decisions.

### For Error/Patch Deduplication Implementation: What Needs Enhancement

The current KB system provides basic content-addressed deduplication for patches but lacks sophisticated similarity detection that could dramatically improve storage efficiency and recommendation quality. The existing signature generation is binary - either error text normalizes to identical content or it's treated as completely different, missing opportunities to group related errors that differ only in variable names, specific values, or minor environmental details.

**Error Signature Similarity Detection:**
Current signatures use exact hash matching after normalization, but similar errors across different codebases often have subtle differences in variable names, package structures, or error contexts. We need fuzzy matching algorithms that can detect when errors are semantically similar even if lexically different. This could use techniques like Levenshtein distance on normalized error text, token-based similarity scoring, or embedding-based semantic similarity.

**Patch Similarity and Semantic Grouping:**
The current patch fingerprinting stores each normalized patch separately, but many patches represent similar fixes (e.g., adding the same import statement across different files, similar refactoring patterns, or equivalent fixes with different variable names). Advanced similarity detection could group patches by their semantic intent rather than just content hash. This might involve analyzing diff patterns, extracting change operations, or computing embeddings of patch semantics.

**Storage Compaction and Merge Operations:**
The current system lacks automated compaction beyond the periodic summary rebuilding mentioned in the roadmap. We need storage compaction that can identify duplicate or near-duplicate cases and merge their statistics intelligently. This includes detecting when multiple error signatures actually represent the same underlying issue and consolidating their case histories.

**Query Performance Optimization:**
The current list-based case reading in `ReadCases()` loads all cases for a signature (up to 1000) into memory for analysis. With advanced deduplication, we could maintain summary statistics incrementally and provide much faster recommendation lookups without full case enumeration.

### Technical Reference Details

#### Current Interface Signatures

```go
type SignatureGenerator interface {
    GenerateSignature(lang, compiler string, stdout, stderr []byte) string
    NormalizePatch(patch []byte) ([]byte, string)
}

type KBStorage interface {
    WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error
    ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error)
    ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error)
    WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error
    StorePatch(ctx context.Context, fingerprint string, patch []byte) error
    GetPatch(ctx context.Context, fingerprint string) ([]byte, error)
}

type SummaryComputer struct {
    storage KBStorage
    lockMgr KBLockManager  
    config  *SummaryConfig
}
```

#### Core Data Structures

```go
type CaseRecord struct {
    RunID     string          `json:"run_id"`
    Timestamp time.Time       `json:"timestamp"`
    Language  string          `json:"language"`
    Signature string          `json:"signature"`
    Context   *CaseContext    `json:"context"`
    Attempt   *HealingAttempt `json:"attempt"`
    Outcome   *HealingOutcome `json:"outcome"`
    BuildLogs *SanitizedLogs  `json:"build_logs,omitempty"`
}

type PromotedFix struct {
    Kind          string    `json:"kind"` // "orw_recipe" or "patch_fingerprint"
    Ref           string    `json:"ref"`  // recipe name or patch fingerprint
    Score         float64   `json:"score"`
    Wins          int       `json:"wins"`
    Failures      int       `json:"failures"`
    LastSuccessAt time.Time `json:"last_success_at"`
    FirstSeenAt   time.Time `json:"first_seen_at"`
}
```

#### Configuration Parameters

```go
type SummaryConfig struct {
    MinCasesForPromotion  int     // 3
    MinSuccessRate        float64 // 0.6
    MaxPromotedFixes      int     // 10
    RecencyWeight         float64 // 0.3
    FrequencyWeight       float64 // 0.4
    SuccessRateWeight     float64 // 0.3
    MinScore              float64 // 0.5
    PromotionLookbackDays int     // 90
}
```

#### Storage Key Patterns

- Case storage: `SeaweedFSKBStorage.buildCaseKey(lang, signature, runID)` → `kb/healing/errors/{lang}/{signature}/cases/{runID}.json`
- Summary storage: `SeaweedFSKBStorage.buildSummaryKey(lang, signature)` → `kb/healing/errors/{lang}/{signature}/summary.json` 
- Patch storage: `SeaweedFSKBStorage.buildPatchKey(fingerprint)` → `kb/healing/patches/{fingerprint}.patch`
- Lock keys: `BuildSignatureLockKey(lang, signature)` → `{lang}/{signature}` (under `kb/locks/`)

#### Implementation Locations

- **Deduplication algorithms**: New implementations in `internal/cli/transflow/kb_signatures.go`
- **Storage compaction logic**: Extensions to `internal/cli/transflow/kb_storage.go` and new compaction module
- **Enhanced summary computation**: Extensions to `internal/cli/transflow/kb_summary.go`
- **Database/index structures**: Potentially new files in `internal/cli/transflow/` for similarity indices
- **Tests**: `internal/cli/transflow/kb_*_test.go` files following existing patterns
- **Configuration**: Extensions to `KBConfig` in `internal/cli/transflow/kb_integration.go`

## User Notes
This builds on the recently completed KB persistence layer to add intelligent deduplication. Focus on storage efficiency and query performance improvements while maintaining the existing API contracts.

## Work Log
<!-- Updated as work progresses -->
- [2025-09-05] Task created following completion of basic KB persistence layer