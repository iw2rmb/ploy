# Observability Metrics

## Why
- Control plane must publish Prometheus metrics for queue depth, claim latency, retries, and SHIFT duration.
- Operators need scrape guidance aligned with Ploy v2 docs and alert suggestions.

## What to do
- Instrument control plane and scheduler components to emit Prometheus metrics covering queue depth, claim latency, retries, and SHIFT duration.
- Document scrape configuration and sample alerts in operator docs.
- Feed GC and retention metrics from [`../gc-audit-metrics/README.md`](../gc-audit-metrics/README.md) once implemented.

## Where to change
- [`internal/controlplane/httpapi`](../../../internal/controlplane/httpapi) to expose `/metrics`.
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler) for instrumentation hooks.
- [`internal/metrics`](../../../internal/metrics) or equivalent for metric registration.
- [`docs/v2/observability.md`](../../v2/observability.md) or related docs to describe scrape setup.

## COSMIC evaluation
| Functional process         | E | X | R | W | CFP |
|----------------------------|---|---|---|---|-----|
| Expose Prometheus metrics  | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                  | 1 | 1 | 1 | 0 | 3   |

- Assumption: metrics registry already exists; work adds metric definitions and HTTP handler wiring.
- Open question: confirm SHIFT duration metric definition (start/stop markers) across scheduler and runtime.

## How to test
- `go test ./internal/controlplane/scheduler -run TestMetrics` verifying counters and histograms register.
- `curl` scrape of `/metrics` in integration environment to ensure metrics emit with labels.
- Documentation review: ensure alert examples validate via `promtool`.
