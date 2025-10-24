# Reusing Grid Components in Ploy Next

Ploy Next benefits from selectively porting mature modules from the Grid repository. This document
highlights the primary candidates, why they are useful, and where to find them.

## Beacon & Discovery

- **HTTP Service & DNS manager** — `../grid/internal/beacon/http`, `../grid/internal/beacon/dns`,
  `../grid/internal/beacon/store`. These packages already expose the REST API, DNS updates, and storage
  abstraction needed for beacon mode.
- **gridbeacon entrypoint** — `../grid/cmd/gridbeacon/main.go`. Provides configuration parsing and
  lifecycle wiring that can be adapted to Ploy’s etcd-backed beacon.
- **GridCTL client** — `../grid/internal/gridctl/beacon`. Reuse the caching client, registration
  helpers, resolver writer, and metadata version checks so `ploy cluster connect` can detect stale
  configuration (mirroring Grid’s metadata refresh flow).

## Job Runtime & Telemetry

- **Job service** — `../grid/internal/jobs`. Contains the job store interface, runtime adapter
  integration, and event publishing hooks. Copy/adapt to run against etcd instead of JetStream.
- **Docker runtime adapter** — `../grid/internal/runtime/docker`. Implements container lifecycle
  operations, log retrieval, and secret mounting.
- **Runtime registry** — `../grid/internal/runtime`. Provides adapter registration and metadata that
  Ploy can reuse for future alternative schedulers.
- **Log streaming helpers** — `../grid/internal/jobs/service_runtime.go` and related publishers
  provide the SSE/tailing behaviour that Ploy exposes through `ploy logs job` and node log endpoints.

## Artifact & Snapshot Handling

- **IPFS publisher** — `../grid/internal/workflow/snapshots` and `../grid/internal/registry/store`.
  While Ploy pivots to IPFS Cluster and etcd, reuse publishing helpers and IPFS interaction logic
  where applicable.
- **Contracts & subjects** — `../grid/internal/workflow/contracts`. Provides consistent schema
  definitions for checkpoints, artifacts, and ticket subjects.

## CLI Support & Automation

- **Grid runtime adapter wiring** — `../grid/cmd/gridctl` and `../grid/internal/gridctl/*`. Useful
  references for CLI UX patterns, configuration handling, state caching, and metadata version tags.
- **Bootstrap scripts** — `../grid/cmd/gridctl/cluster` packages house SSH/deploy helpers that can
  inform the shared bootstrap script embedded in the Ploy CLI.
- **Tests & fakes** — Look at `../grid/internal/jobs/service_*_test.go`, `../grid/internal/beacon/store/memory_test.go`,
  and related stubs to accelerate Ploy’s test scaffolding.

## How to Port

1. **Identify dependencies** — Many modules rely on Grid-specific configuration (JetStream,
   Cloudflare). Strip or replace with Ploy equivalents (etcd, IPFS Cluster).
2. **Keep interfaces** — Preserve public interfaces where possible so downstream code (CLI,
   workflow runner) requires minimal changes.
3. **Document differences** — Note behavioural differences (e.g., SSE streams replacing JetStream,
   metadata version tags stored alongside cluster descriptors) in `docs/next` so future contributors
   understand the fork.
4. **Validate with tests** — Reuse Grid’s unit/integration tests as a starting point, adjusting for
   Ploy’s environment.

Reusing these components accelerates Ploy Next without sacrificing stability, while still allowing the
system to move away from Grid’s infrastructure assumptions.
