# Observability Log Bundles

## Why
- Inspection-ready jobs need archived log bundles in IPFS Cluster with predictable retention windows.
- Deduplicated writes and failure retries must preserve bundles despite network instability.

## What to do
- Persist log bundles to IPFS Cluster with deduplicated writes, retry policies, and CID tracking.
- Record retention metadata alongside job summaries so CLI and control plane can surface bundle details (see [`../observability-retention-cli/README.md`](../observability-retention-cli/README.md)).
- Handle pin failures with alerting hooks for observability metrics in [`../observability-metrics/README.md`](../observability-metrics/README.md).

## Where to change
- [`internal/workflow/runtime/step`](../../../internal/workflow/runtime/step) for publishing bundles and capturing CIDs.
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler) to coordinate bundle persistence with job lifecycle.
- [`internal/ipfs`](../../../internal/ipfs) or equivalent for cluster client retries and dedupe.
- [`docs/v2/logs.md`](../../v2/logs.md) to describe bundle retention behaviour.

## COSMIC evaluation
| Functional process                    | E | X | R | W | CFP |
|---------------------------------------|---|---|---|---|-----|
| Persist log bundles to IPFS Cluster   | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                             | 1 | 1 | 1 | 1 | 4   |

- Assumption: dedupe uses existing CID index; no new persistent store beyond job summary updates.
- Open question: confirm IPFS Cluster version compatibility with planned retry semantics.

## How to test
- `go test ./internal/workflow/runtime/step -run TestLogBundles` verifying CID tracking and retry flows.
- Integration: execute Mod and inspect pinned bundle via IPFS Cluster API.
- Smoke: run GC dry run to confirm bundles marked for retention until TTL expiry.
