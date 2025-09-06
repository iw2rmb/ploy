# Knowledge Base (KB) CLAUDE.md

## Purpose
Foundational data layer for the transflow MVP self-healing Knowledge Base system that stores error patterns, successful patches, and learning summaries to improve automated error resolution success rates.

## Architecture Overview
The KB system consists of four core modules that work together to learn from build failures and provide intelligent patch recommendations:

- **Models**: Core data structures (Error, Case, Summary)
- **Storage**: SeaweedFS-backed persistence layer
- **Fingerprint**: Patch analysis and similarity detection
- **Learning**: Orchestration and learning pipeline

## Module Structure
- `models/` - Core KB data models
  - `error.go:11-97` - Error pattern representation and normalization
  - `case.go:11-78` - Learning case with patch data and confidence
  - `summary.go:9-118` - Aggregated learning statistics
- `storage/` - Persistence layer
  - `config.go:5-9` - Storage configuration
  - `kb_storage.go:15-181` - SeaweedFS storage operations
- `fingerprint/` - Patch analysis
  - `patch.go:10-202` - Pattern extraction and similarity analysis
- `learning/` - Learning orchestration
  - `learner.go:14-282` - Main learning pipeline and recommendations

## Key Components

### Models (models/)

#### Error (error.go)
- `Error:12-19` - Canonical error pattern with signature generation
- `NewError:22-34` - Constructor with automatic signature generation
- `GenerateSignature:37-51` - Creates canonical signatures for error patterns
- `NormalizeMessage:54-66` - Normalizes error messages for pattern matching

#### Case (case.go)
- `Case:12-20` - Single learning instance linking error to patch outcome
- `NewCase:23-36` - Constructor with UUID and patch hash generation
- `UpdateConfidence:49-64` - Confidence calculation from historical data

#### Summary (summary.go)
- `Summary:10-16` - Aggregated statistics for error patterns
- `PatchSummary:19-24` - Statistics for specific patch patterns
- `CalculateSuccessRate:27-45` - Success rate calculation from cases
- `GenerateTopPatches:48-93` - Top-performing patch identification

### Storage (storage/)

#### KBStorage (kb_storage.go)
- `KBStorage:16-33` - SeaweedFS HTTP client wrapper
- `StoreError:36-39` - Error persistence with signature-based keys
- `RetrieveError:42-49` - Error retrieval by signature
- `StoreCase:52-55` - Case persistence with UUID keys
- `ListCasesByError:68-89` - Case retrieval filtered by error ID
- `StoreSummary:92-95` - Summary persistence

### Fingerprinting (fingerprint/)

#### PatchFingerprinter (patch.go)
- `PatchFingerprinter:11-20` - Pattern-based patch analysis
- `GenerateFingerprint:23-41` - Semantic fingerprint generation
- `NormalizePatch:44-67` - Patch normalization for consistency
- `ExtractPatterns:70-120` - Semantic pattern extraction from patches
- `CalculateSimilarity:123-160` - Pattern-based similarity scoring

### Learning (learning/)

#### KBLearner (learner.go)
- `KBLearner:15-52` - Main learning pipeline orchestration
- `LearnFromError:55-80` - Primary learning entry point
- `processPatchCase:83-113` - Patch case processing with deduplication
- `GetBestPatch:238-274` - High-confidence patch recommendation
- `updateErrorSummary:161-183` - Summary statistics maintenance

## Integration Points

### Consumes
- SeaweedFS HTTP API for distributed storage
- Build error messages and logs from transflow pipeline
- Git patch data from automated healing attempts

### Provides
- Error pattern recognition and canonicalization
- Patch similarity analysis and deduplication
- Success rate statistics and confidence scoring
- Best patch recommendations for known error patterns

## Configuration

### Storage Configuration
- `Config.StorageURL` - SeaweedFS master/volume server endpoint
- `Config.Timeout` - HTTP request timeout (default: 30s)

### Learning Configuration
- `Config.MinConfidence` - Minimum confidence threshold (default: 0.7)
- `Config.MaxCasesPerError` - Case limit per error pattern (default: 50)
- `Config.SimilarityThreshold` - Patch deduplication threshold (default: 0.8)
- `Config.EnableDebugLogging` - Debug output control

## Storage Schema

### Key Patterns
- `kb/errors/{signature}` - Error patterns by canonical signature
- `kb/cases/{uuid}` - Individual learning cases
- `kb/summaries/{error_id}` - Aggregated error statistics

### Data Flow
1. Build errors normalized and stored with canonical signatures
2. Patches fingerprinted and stored as cases linked to errors
3. Similar patches deduplicated using semantic analysis
4. Success rates and confidence scores calculated from historical data
5. Best patches recommended based on confidence thresholds

## Key Patterns

### Error Canonicalization
- Signature generation from normalized error messages
- Pattern-based classification (java-compilation-missing-symbol, java-syntax-semicolon)
- Build log normalization and filtering

### Patch Fingerprinting
- Semantic pattern extraction (import additions, syntax fixes, Optional wrappers)
- Content normalization (variable name generalization, path simplification)
- Similarity scoring based on pattern overlap

### Learning Pipeline
- Deduplication of similar patches using configurable thresholds
- Confidence scoring based on historical success rates
- Summary statistics maintenance for performance tracking

## Testing
- Test files: `*_test.go` in each module directory
- Run tests: Reference in root Makefile or go test commands
- Coverage: Focus on model validation, storage operations, and pattern matching

## Related Documentation
- `roadmap/transflow/README.md` - Overall transflow MVP architecture
- Root `CLAUDE.md` - Project-wide development protocols
- Storage backend documentation in SeaweedFS docs