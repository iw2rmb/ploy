# Observability Log Streaming

## Why
- Operators require real-time job logs via SSE to debug Mods during execution.
- Streaming must work uniformly across control plane and node services.

## What to do
- Implement `/v2/jobs/{id}/logs/stream` SSE endpoints in control plane and node services with backpressure-aware buffers.
- Ensure stream includes retention hints consumed by CLI (see [`../observability-retention-cli/README.md`](../observability-retention-cli/README.md)).
- Document the API contract for CLI consumers in [`../cli-streaming/README.md`](../cli-streaming/README.md).

## Where to change
- [`internal/controlplane/httpapi`](../../../internal/controlplane/httpapi) for SSE handlers and cancellation support.
- [`internal/workflow/runtime/step`](../../../internal/workflow/runtime/step) to publish log events to the stream.
- [`internal/node/logstream`](../../../internal/node/logstream) or equivalent for node relay buffers.
- [`docs/v2/logs.md`](../../v2/logs.md) to capture streaming usage.

## COSMIC evaluation
| Functional process           | E | X | R | W | CFP |
|------------------------------|---|---|---|---|-----|
| Stream live job logs via SSE | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                    | 1 | 1 | 1 | 0 | 3   |

- Assumption: SSE buffers reside in memory only; no persistent writes.
- Open question: confirm log ordering requirements across distributed nodes.

## How to test
- `go test ./internal/controlplane/httpapi -run TestLogsStream` to cover SSE connect, stream, cancel.
- Integration: run Mod and verify streaming across node and control plane surfaces.
- Smoke: `make build && dist/ploy mods logs <job-id>` to validate CLI end-to-end once CLI doc is implemented.
