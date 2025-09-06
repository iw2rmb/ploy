# Storage Module CLAUDE.md

## Purpose
Unified storage interface providing abstraction over SeaweedFS with comprehensive monitoring, retry logic, and integrity verification for artifact management and Knowledge Base deduplication operations.

## Narrative Summary
The storage module provides a clean, unified interface for object storage operations while abstracting the underlying SeaweedFS implementation. It handles storage provider abstraction, error recovery through sophisticated retry mechanisms, integrity verification for uploaded artifacts, and comprehensive monitoring of storage operations.

Core functionality centers around the Storage interface which provides basic CRUD operations, batch operations, metadata management, and health monitoring. The SeaweedFS implementation includes advanced features like artifact bundle uploads with SBOM/signature handling, configurable retry policies with exponential backoff, and real-time metrics collection.

**KB Deduplication Integration**: The storage layer serves as the backend for the transflow Knowledge Base deduplication system, providing high-performance storage for error signatures, healing patches, case summaries, and deduplication metadata. It supports the compaction operations that achieve 50%+ storage reduction through intelligent case merging and automated cleanup of redundant data.

## Key Files
- `interface.go:70-93` - Core Storage interface with CRUD and batch operations
- `interface.go:97-122` - Legacy StorageProvider interface for compatibility
- `seaweedfs.go:1-600` - Complete SeaweedFS client implementation
- `client.go:1-400` - HTTP client with connection pooling and timeout handling
- `retry.go:1-250` - Sophisticated retry logic with exponential backoff and circuit breaker
- `errors.go:1-300` - Comprehensive error handling with classification and recovery strategies
- `integrity.go:1-220` - Artifact integrity verification with checksum validation
- `monitoring.go:1-500` - Real-time metrics collection and health monitoring
- `adapter.go:1-150` - Storage provider adapter for legacy compatibility
- `storage.go:1-80` - Package initialization and configuration
- `llm_models.go:1-319` - LLM model registry storage operations with CRUD, filtering, search, backup/restore

## Integration Points
### Consumes
- SeaweedFS HTTP API: Object storage operations (GET, PUT, DELETE, LIST)
- Prometheus Metrics: Storage operation metrics and health indicators
- System Resources: File system access for artifact bundle processing

### Provides
- Storage Interface: Core storage abstraction (storage.Storage)
- StorageProvider Interface: Legacy artifact upload interface
- Metrics API: Storage operation metrics via StorageMetrics
- Health Monitoring: Storage backend health checks
- Integrity Verification: Artifact checksum validation and bundle verification
- KB Storage Backend: High-performance storage for Knowledge Base deduplication operations
- Compaction Support: Storage operations optimized for deduplication case merging and cleanup
- LLM Model Storage: Registry operations for LLM models under `llms/models/` namespace with filtering, search, backup/restore

## Configuration
Environment variables:
- `SEAWEEDFS_MASTER` - SeaweedFS master server URL
- `SEAWEEDFS_VOLUME` - Volume server URL for direct operations  
- `SEAWEEDFS_BUCKET` - Default bucket name
- `STORAGE_TIMEOUT` - Operation timeout (default: 30s)
- `STORAGE_RETRY_MAX` - Maximum retry attempts (default: 3)
- `STORAGE_RETRY_DELAY` - Initial retry delay (default: 1s)

## Key Patterns
- Interface-based abstraction with pluggable backend implementations (see interface.go:70-93)
- Comprehensive retry policies with exponential backoff (see retry.go:80-150)
- Error classification and recovery strategies (see errors.go:50-150)
- Real-time metrics collection with Prometheus integration (see monitoring.go:100-300)
- Integrity verification with multi-stage validation (see integrity.go:50-120)
- Connection pooling and resource management (see client.go:100-200)
- Graceful degradation with health monitoring (see seaweedfs.go:450-500)

## Related Documentation
- `../cli/transflow/CLAUDE.md` - Transflow KB persistence and deduplication integration
- `../../api/storage/` - Storage API endpoints and handlers
- `../../api/llms/` - LLM model registry API endpoints
- `../../platform/seaweedfs/` - SeaweedFS deployment and configuration
- `../orchestration/CLAUDE.md` - Orchestration layer supporting KB maintenance jobs
- `../../cmd/ployman/` - CLI tool for LLM model management