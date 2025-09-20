# Storage

## Key Takeaways
- Provides a unified storage interface for artifact bundles, SBOM/signature assets, and Knowledge Base payloads backed by SeaweedFS.
- Handles retries, integrity checks, and metrics so higher-level packages can treat storage as a robust, observable service.
- Exposes both modern `Storage` and legacy provider interfaces, allowing APIs/CLI/Mods to migrate without breaking older code paths.

## Feature Highlights
- **Storage Interface (`Storage`)**: CRUD operations, streaming uploads, batch deletes, metadata helpers, and health checks used across builds, mods, and KB jobs.
- **SeaweedFS Client**: HTTP client with connection pooling, retry policies, circuit breaker logic, and namespace helpers for artifacts and KB structures.
- **Integrity & Verification**: Checksum validation, SBOM/signature verification, and artifact bundle inspection before success responses are returned.
- **Metrics & Monitoring**: Prometheus instrumentation for latency, throughput, error rates; health probes for readiness/liveness.
- **Compatibility Layer**: Legacy `StorageProvider` adapter so older modules still work while new functionality migrates to the modern API.

## Package Layout
- `interface.go` – Defines the `Storage` interface, legacy `StorageProvider`, base types, and helper constructors.
- `seaweedfs.go` – Production SeaweedFS implementation (initialisation, read/write/delete, health checks, namespace helpers).
- `client.go` – Shared HTTP client with pooling, timeouts, and request helpers.
- `retry.go` / `errors.go` – Retry backoff strategies, error classification, and circuit breaker state.
- `integrity.go` – Artifact verification (checksums, SBOM/signature validation) and bundle inspection.
- `monitoring.go` – Prometheus metrics collectors and health gauge updates.
- `storage.go` – Package wiring (default constructors, configuration hooks).
- `llm_models.go` – LLM model registry persistence (CRUD/filter/export) reusing the Storage abstractions.
- `adapter.go` – Bridges new storage interface to legacy provider implementations.

## Usage Notes
- Prefer the `Storage` interface for new code; use `NewSeaweedFSStorage` (or DI wrapper) to obtain a production client.
- For bulk uploads (artifacts, SBOM, signatures), call `Put`, `PutStream`, or helper wrappers to ensure integrity checks run.
- When running in mods or healing flows, leverage the storage client together with orchestration locks (`internal/orchestration/README.md`) for safe compaction/snapshot jobs.
- Metrics/health endpoints rely on the storage client—ensure `RegisterMetrics()` is called during service boot.

## Related Docs
- `internal/orchestration/README.md` – Shows how storage is combined with Consul/Nomad for KB maintenance jobs.
- `internal/build/README.md` – Describes artifact bundle uploads and verifications that depend on storage.
- `platform/README.md` – Deployment/runtime settings for SeaweedFS in each lane.
