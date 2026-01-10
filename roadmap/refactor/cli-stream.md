# CLI Stream Refactor Notes (`internal/cli/stream`)

This file tracks remaining `internal/cli/stream` runtime correctness and cleanup.

## Streamlining / Simplification

- Remove duplicated implementations (`Client` vs `SSEClient`).
  - Today `internal/cli/stream/client.go` and `internal/cli/stream/sse_client.go` implement near-identical reconnect/idle logic; production call sites use `Client` only.
  - Solution: keep a single implementation and delete the other (plus tests) to reduce divergence.
- Unify backoff logic.
  - Today `Client` uses the shared backoff policy (`internal/workflow/backoff`) while `SSEClient` has a custom jitter/backoff implementation (`internal/cli/stream/sse_client.go:304`).
  - Solution: use one backoff policy implementation for all CLI streaming code.

## Likely Bugs / Risks

- Idle-timeout timer can cancel the wrong connection.
  - `time.AfterFunc(... cancelConn())` captures the loop variable; a timer firing late can call the cancel func for a later iteration (`internal/cli/stream/client.go:120`, `internal/cli/stream/sse_client.go:129`).
  - Solution: capture the cancel func value for the closure (`cancel := cancelConn`) and ensure per-connection timers are stopped/drained before reconnect.
- Per-connection cancels are deferred inside a reconnect loop.
  - `defer cancelConn()` is inside `for {}` (`internal/cli/stream/client.go:63`, `internal/cli/stream/sse_client.go:70`), so cancels accumulate until the stream returns.
  - Solution: call `cancelConn()` at the end of each iteration (after closing the response body / stopping timers), not via `defer`.
- Context cancellation is misreported as idle timeout when `IdleTimeout > 0`.
  - On `Do`/read errors, any `connCtx.Err()!=nil` becomes “idle timeout” if `IdleTimeout>0`, even if the parent context was canceled (`internal/cli/stream/client.go:89`, `internal/cli/stream/sse_client.go:96`).
  - Solution: track whether the idle timer fired (a boolean/atomic) and only return idle-timeout in that case; otherwise return `ctx.Err()`.
- Comments claim server `retry:` hints are respected, but the implementation cannot read them.
  - Both clients use `go-sse` `Read`, which does not expose the `retry` field; `Event.Retry` is always zero (`internal/cli/stream/client.go:180`, `internal/cli/stream/sse_client.go:193`).
  - Solution: either (a) remove retry-hint claims from docstrings and the `Retry` field, or (b) switch to a `go-sse` API that exposes retry hints and implement them.
- Missing `Cache-Control: no-cache` on the main `Client`.
  - `SSEClient` sets it; `Client` does not (`internal/cli/stream/sse_client.go:82`, `internal/cli/stream/client.go:76`).
  - Solution: set `Cache-Control: no-cache` in `Client` as well to reduce proxy buffering risk.
- `MaxEventSize` is hard-coded to 1 MiB.
  - This can truncate large log frames (stack traces, big JSON payloads) (`internal/cli/stream/client.go:136`, `internal/cli/stream/sse_client.go:143`).
  - Solution: make max event size configurable (CLI flag / env) and document the server-side chunking expectation.

## Suggested Minimal Slices

- Slice 1: Fix idle-timeout cancellation correctness (closure capture, per-iteration cancels, and correct error classification).
- Slice 2: Apply the unified SSE/log payload contract types (merged slice).
- Slice 3: Delete the unused/duplicate SSE client implementation.
