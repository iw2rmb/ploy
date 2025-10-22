# GC Audit Metrics

## Why
- GC requires audit logs and Prometheus metrics to track deletions, failures, and skipped items.
- Operators need dashboards and alerts for storage pressure.

## What to do
- Emit structured audit logs for each GC action, including overrides and retry failures.
- Publish Prometheus metrics (success, failure, skipped counts) consumed by [`../observability-metrics/README.md`](../observability-metrics/README.md).
- Provide dashboard and alert recommendations in operator docs.

## Where to change
- [`internal/controlplane/gc`](../../../internal/controlplane/gc) to log actions and register metrics.
- [`internal/metrics`](../../../internal/metrics) for metric definitions.
- [`docs/v2/gc.md`](../../v2/gc.md) and [`docs/v2/observability.md`](../../v2/observability.md) for dashboards and alerts.
- Upstream dependencies: [`../gc-marker-controller/README.md`](../gc-marker-controller/README.md) and [`../gc-artifact-cleanup/README.md`](../gc-artifact-cleanup/README.md).

## COSMIC evaluation
| Functional process                     | E | X | R | W | CFP |
|----------------------------------------|---|---|---|---|-----|
| Publish GC audit logs and metrics      | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                              | 1 | 1 | 1 | 0 | 3   |

- Assumption: audit logs reuse existing logging pipeline.
- Open question: confirm metrics require cardinality limits per job type or environment.

## How to test
- `go test ./internal/controlplane/gc -run TestMetrics` verifying counters and log emission.
- Integration: run GC cycle, collect logs/metrics, ensure dashboards populate.
- Smoke: `promtool check rules` on provided alert examples.
