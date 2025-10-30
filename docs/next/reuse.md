# Reusing Legacy Components in Ploy Next

Ploy Next benefits from selectively porting mature modules from the legacy repository. This document
highlights the primary candidates, why they are useful, and where to find them.

## Discovery & Metadata

- **Descriptor caches** — Reuse ideas from `../grid/internal/gridctl` for caching discovery metadata,
  but the SSH descriptor files have replaced the dedicated beacon services. Focus on the state caching
  patterns rather than the HTTP surface.

## Job Runtime & Telemetry

- **Job service** — `../grid/internal/jobs`. Contains the job store interface, runtime adapter
  integration, and event publishing hooks. Copy/adapt to run against etcd and SSE streams instead of JetStream.
- **Docker runtime adapter** — `../grid/internal/runtime/docker`. Implements container lifecycle
  operations, log retrieval, and secret mounting.
- **Runtime registry** — `../grid/internal/runtime`. Provides adapter registration and metadata that
  Ploy can reuse for future alternative schedulers.
- **Log streaming helpers** — `../grid/internal/jobs/service_runtime.go` and related publishers
  provide the SSE/tailing behaviour that Ploy exposes through `ploy logs job` and node log endpoints.

## Artifact Handling

- **IPFS publisher** — `../grid/internal/registry/store`.
  While Ploy pivots to IPFS Cluster and etcd, reuse publishing helpers and IPFS interaction logic
  where applicable. Snapshot-specific helpers are no longer applicable.
- **Contracts & subjects** — `../grid/internal/workflow/contracts`. Provides consistent schema
  definitions for checkpoints, artifacts, and ticket subjects.

## CLI Support & Automation

- **Legacy runtime adapter wiring** — `../grid/cmd/gridctl` and `../grid/internal/gridctl/*`. Useful
  references for CLI UX patterns, configuration handling, state caching, and metadata version tags.
- **Bootstrap scripts** — `../grid/cmd/gridctl/cluster` packages house SSH/deploy helpers that can
  inform the shared bootstrap script embedded in the Ploy CLI.
- **Tests & fakes** — Look at `../grid/internal/jobs/service_*_test.go` and related stubs to accelerate
  Ploy’s test scaffolding.

## How to Port

1. **Identify dependencies** — Many modules rely on legacy-specific configuration (JetStream,
   Cloudflare). Strip or replace with Ploy equivalents (etcd, IPFS Cluster).
2. **Keep interfaces** — Preserve public interfaces where possible so downstream code (CLI,
   workflow runner) requires minimal changes.
3. **Document differences** — Note behavioural differences (e.g., SSE streams replacing JetStream,
   metadata version tags stored alongside cluster descriptors) in `docs/next` so future contributors
   understand the fork.
4. **Validate with tests** — Reuse legacy unit/integration tests as a starting point, adjusting for
   Ploy’s environment.

Reusing these components accelerates Ploy Next without sacrificing stability, while still allowing the
system to move away from legacy infrastructure assumptions.
