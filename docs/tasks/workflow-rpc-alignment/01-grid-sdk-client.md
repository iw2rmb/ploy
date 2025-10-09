# Grid SDK Client Replacement

- [x] Completed 2025-09-28

## Status

- `internal/workflow/grid/client.go` now talks directly to
  `github.com/iw2rmb/grid/sdk/workflowrpc/go`, with an injectable factory so
  tests can supply fakes.
- The client preserves invocation tracking, constructs helper-backed SDK
  instances when grid credentials (`PLOY_GRID_ID`, `PLOY_GRID_API_KEY`) are set, and converts runner stages into
  `workflowsdk.SubmitRequest` payloads.
- Unit tests cover payload construction, streaming terminal events, and error
  propagation without relying on the removed `internal/workflow/grid/workflowrpc`
  shim.

## Why / What For

Adopt the official Grid Workflow RPC SDK in `internal/workflow/grid` so Ploy
submits workflow runs through `/v1/workflows/rpc/runs` instead of the deprecated
`/workflow/stages` endpoint.

## Required Changes

- Remove the bespoke HTTP client and replace it with a thin wrapper around
  `workflowsdk.Client` from `grid/sdk/workflowrpc/go` (mirroring the
  configuration patterns documented in `../grid/sdk/workflowrpc/README.md`).
- Ensure the wrapper keeps invocation tracking for CLI summaries and supports
  dependency injection in tests.
- Update configuration paths to surface authentication/token requirements when
  targeting live Grid endpoints.

## Definition of Done

- The new wrapper constructs `workflowsdk.SubmitRequest` payloads using SDK
  types and handles submission/cancellation/streaming via the SDK.
- Tests cover success, non-2xx responses, and stream handling errors.
- CLI wiring and dependency factories instantiate the SDK client when
  grid credentials are provided.
- Wrapper exports hooks so Roadmap 04 can swap in the helper once it lands
  without refactoring call sites.

## Tests

- Unit tests for the wrapper verifying request payloads and error translation.
- CLI tests ensuring grid credentials trigger SDK-based execution with
  invocation tracking preserved.

## References

- Ploy Workflow RPC Alignment design
  (`docs/design/workflow-rpc-alignment/README.md`).
- Grid Workflow RPC SDK (`../grid/sdk/workflowrpc/go`).
- Grid Workflow RPC helper guidance (`../grid/sdk/workflowrpc/README.md`).
