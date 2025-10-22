# roadmap-observability-07 — Job Logs & Metrics

- **Status**: Planned — 2025-10-22
- **Dependencies**: `docs/design/observability/README.md` (to be authored), `docs/v2/logs.md`, `docs/v2/gc.md`

## Why

- Operators need real-time job logs, archival guarantees, and metrics to diagnose Mods runs on both
  workstations and clusters.
- The control plane must expose log streaming, retention metadata, and Prometheus metrics aligned
  with the v2 documentation.

## What to do

- Implement `/v2/jobs/{id}/logs/stream` SSE endpoints on control plane and node services with
  backpressure-aware buffers.
- Persist log bundles to IPFS Cluster with retention TTLs and expose summary metadata in job records.
- Emit Prometheus metrics (queue depth, claim latency, retries, SHIFT duration) and document scrape
  configuration.

## Where to change

- `internal/controlplane/httpapi` — SSE log handler, retention metadata surfacing.
- `internal/controlplane/scheduler` — metrics instrumentation, GC marker wiring.
- `internal/workflow/runtime/step` — log bundle publication and retention TTL propagation.
- `docs/v2/logs.md`, `docs/v2/testing.md` — update operator guidance and test expectations.

## How to test

- `go test ./internal/controlplane/httpapi` with SSE-focused tests (time-bounded log streaming).
- Integration: run a Mod, confirm logs stream live and archived bundle pins in IPFS.
- Metrics scrape smoke: expose `/metrics`, validate queue depth and retry counters via Prometheus
  integration tests.
