# GC Artifact Cleanup

## Why
- Storage pressure requires automated cleanup of expired artifacts, containers, and diff bundles.
- Cleanup must retry safely and avoid deleting inspection-ready jobs.

## What to do
- Execute cleanup workers that unpin IPFS artifacts, prune node containers, and remove job state with retry guards.
- Respect staging decisions from [`../gc-marker-controller/README.md`](../gc-marker-controller/README.md).
- Publish failure signals consumed by [`../gc-audit-metrics/README.md`](../gc-audit-metrics/README.md).

## Where to change
- [`internal/controlplane/gc`](../../../internal/controlplane/gc) to run cleanup workers.
- [`internal/ipfs`](../../../internal/ipfs) for unpin operations and retries.
- [`internal/node/runtime`](../../../internal/node/runtime) or similar for container prune RPCs.
- [`docs/v2/gc.md`](../../v2/gc.md) to detail cleanup actions.

## COSMIC evaluation
| Functional process                         | E | X | R | W | CFP |
|--------------------------------------------|---|---|---|---|-----|
| Execute artifact and container cleanup     | 1 | 1 | 0 | 2 | 4   |
| **TOTAL**                                  | 1 | 1 | 0 | 2 | 4   |

- Assumption: cleanup writes occur for deletion markers only; no extra metadata persisted.
- Open question: confirm container prune RPC needs new authentication hooks post-rotation.

## How to test
- `go test ./internal/controlplane/gc -run TestCleanupWorker` covering retries and failure handling.
- Integration: expire job, verify artifact unpin and container prune across nodes.
- Smoke: run `make build && dist/ploy gc --dry-run` to ensure cleanup preview lists staged deletions.
