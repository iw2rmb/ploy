# GC CLI Overrides

## Why
- Operators need manual overrides to accelerate cleanup or protect jobs from deletion.
- CLI should support dry runs, per-job retention adjustments, and reporting on deletions versus failures.

## What to do
- Add CLI commands for manual sweeps, dry-run previews, and retention overrides.
- Ensure overrides feed into marker controller per [`../gc-marker-controller/README.md`](../gc-marker-controller/README.md).
- Display audit summaries sourced from [`../gc-audit-metrics/README.md`](../gc-audit-metrics/README.md).

## Where to change
- [`cmd/ploy/gc`](../../../cmd/ploy/gc) for new CLI flags and output formatting.
- [`internal/controlplane/gc`](../../../internal/controlplane/gc) to apply manual overrides.
- [`docs/v2/gc.md`](../../v2/gc.md) documenting CLI usage.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) for snapshot updates.

## COSMIC evaluation
| Functional process                 | E | X | R | W | CFP |
|------------------------------------|---|---|---|---|-----|
| Run manual CLI sweeps and overrides| 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                          | 1 | 1 | 1 | 1 | 4   |

- Assumption: overrides persist in etcd with TTLs to avoid permanent retention changes.
- Open question: confirm dry-run output must include diff bundles or summary counts only.

## How to test
- `go test ./cmd/ploy/gc -run TestOverrides` covering CLI inputs and API calls.
- Integration: apply override to job, confirm marker controller respects new TTL.
- Smoke: `make build && dist/ploy gc --dry-run` verifying override details.
