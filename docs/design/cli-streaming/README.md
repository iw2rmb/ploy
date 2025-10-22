# CLI Streaming

## Why
- Operators rely on real-time job logs via CLI commands such as `ploy mods logs` and `ploy jobs follow`.
- Streaming UX must align with SSE endpoints delivered by the control plane.

## What to do
- Implement streaming commands that consume SSE endpoints, handle backpressure, and support follow/retry semantics.
- Expose formatting options (raw vs. structured) and surface retention hints sourced from [`../observability-log-bundles/README.md`](../observability-log-bundles/README.md).
- Update CLI docs with streaming examples.

## Where to change
- [`cmd/ploy/mods`](../../../cmd/ploy/mods) and related packages for SSE client wiring and output formatting.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) for streaming snapshots.
- [`docs/v2/logs.md`](../../v2/logs.md) to show CLI streaming guidance.
- Upstream design doc: [`../observability-log-streaming/README.md`](../observability-log-streaming/README.md).

## COSMIC evaluation
| Functional process            | E | X | R | W | CFP |
|-------------------------------|---|---|---|---|-----|
| Implement streaming log follow | 1 | 1 | 1 | 0 | 3 |
| **TOTAL**                     | 1 | 1 | 1 | 0 | 3 |

- Assumption: streaming commands buffer in memory only; no persistence writes.
- Open question: decide whether CLI auto-reconnects on SSE disconnect or requires manual retry.

## How to test
- `go test ./cmd/ploy/mods -run TestLogsFollow` using SSE fakes.
- Snapshot tests capturing streaming output under `cmd/ploy/testdata`.
- Smoke: `make build && dist/ploy mods logs <job-id>` against dev control plane to confirm stream fidelity.
