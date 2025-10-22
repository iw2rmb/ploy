# GC Marker Controller

## Why
- GC requires a controller that selects expired markers and stages safe deletion plans.
- Predictable retention windows depend on accurate marker evaluation.

## What to do
- Scan etcd `gc/jobs/<job-id>` markers, validate dependencies, and stage deletion plans.
- Coordinate with retention metadata from [`../observability-retention-cli/README.md`](../observability-retention-cli/README.md).
- Emit staging status for audit metrics in [`../gc-audit-metrics/README.md`](../gc-audit-metrics/README.md).

## Where to change
- [`internal/controlplane/gc`](../../../internal/controlplane/gc) for controller loop logic.
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler) if additional hooks needed for job state.
- [`internal/etcd`](../../../internal/etcd) helpers for marker scans.
- [`docs/v2/gc.md`](../../v2/gc.md) describing marker lifecycle.

## COSMIC evaluation
| Functional process                               | E | X | R | W | CFP |
|--------------------------------------------------|---|---|---|---|-----|
| Select expired GC markers and stage deletions    | 1 | 1 | 2 | 0 | 4   |
| **TOTAL**                                        | 1 | 1 | 2 | 0 | 4   |

- Assumption: staging writes only to in-memory plan; final deletes handled by cleanup doc.
- Open question: confirm marker schema includes retention overrides from CLI.

## How to test
- `go test ./internal/controlplane/gc -run TestMarkerController` covering expiration selection and dependency checks.
- Integration: create markers with varying TTLs, verify controller staging decisions.
- Smoke: enable debug logs to inspect staged plans before cleanup executes.
