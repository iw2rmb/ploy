# Observability

## Why
- Operators need real-time job logs, retention metadata, and Prometheus metrics to debug Mods on both workstations and clusters.
- Log bundles must persist in IPFS Cluster with clear retention windows so inspection-ready jobs stay recoverable.
- The control plane should expose a consistent `/metrics` surface and document scrape guidance aligned with Ploy v2 docs.

## What to do
- Implement `/v2/jobs/{id}/logs/stream` SSE endpoints on control plane and node services with backpressure-aware buffers.
- Persist log bundles to IPFS Cluster, recording CIDs, digests, and retention TTLs in job metadata; expose summary fields to the CLI.
- Add Prometheus metrics (queue depth, claim latency, retries, SHIFT duration) and document scrape configuration plus alert suggestions.

## Where to change
- [`internal/controlplane/httpapi`](../../../internal/controlplane/httpapi) for SSE handlers, log metadata exposure, and metrics endpoints.
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler) for metrics instrumentation and GC marker wiring.
- [`internal/workflow/runtime/step`](../../../internal/workflow/runtime/step) for publishing log bundles and propagating retention settings.
- [`docs/v2/logs.md`](../../v2/logs.md), [`docs/v2/testing.md`](../../v2/testing.md), and related operator docs for streaming guidance and metric references.

## COSMIC evaluation
| Functional process          | E | X | R | W | CFP |
|-----------------------------|---|---|---|---|-----|
| Stream live job logs        | 1 | 1 | 1 | 0 | 3   |
| Persist log bundles         | 1 | 1 | 1 | 2 | 5   |
| Expose Prometheus metrics   | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                   | 3 | 3 | 3 | 2 | 11  |

- Assumptions: log persistence writes cover both IPFS pinning and metadata updates; SSE flow omits retry/error exits from the initial sizing.
- Open questions: confirm whether node-side buffering introduces additional writes for temporary storage.

## How to test
- `go test ./internal/controlplane/httpapi` with SSE-focused tests covering streaming and cancellation.
- Integration runs: execute a Mod, verify live log streaming and archived bundles pinned in IPFS Cluster.
- Metrics smoke: scrape `/metrics`, confirm queue depth and retry counters emit in Prometheus format.
