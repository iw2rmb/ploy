# Grid SDK Client Replacement
- [x] Completed 2025-09-28

## Status
- Wrapped the Workflow RPC endpoint behind `internal/workflow/grid/workflowrpc` with an injectable factory so the runner can consume the SDK surface while tests supply fakes.
- `internal/workflow/grid.Client` now constructs the SDK client when `GRID_ENDPOINT` is set, preserves invocation tracking, and converts runner stages to/from the SDK envelopes.
- Added unit tests for both the SDK wrapper and the runner integration, covering happy-path submissions, error propagation, and workstation skips for root-only permission checks.

## Why / What For
Adopt the official Grid Workflow RPC SDK in `internal/workflow/grid` so Ploy submits workflow runs through `/v1/workflows/rpc/runs` instead of the deprecated `/workflow/stages` endpoint.

## Required Changes
- Remove the bespoke HTTP client and replace it with a thin wrapper around `workflowrpc.Client` from `grid/sdk/workflowrpc/go` (mirroring the configuration patterns documented in `../grid/sdk/workflowrpc/README.md`).
- Ensure the wrapper keeps invocation tracking for CLI summaries and supports dependency injection in tests.
- Update configuration paths to surface authentication/token requirements when targeting live Grid endpoints.

## Definition of Done
- The new wrapper constructs `workflowrpc.SubmitRequest` payloads using SDK types and handles submission/cancellation/streaming via the SDK.
- Tests cover success, non-2xx responses, and stream handling errors.
- CLI wiring and dependency factories instantiate the SDK client when `GRID_ENDPOINT` is provided.
- Wrapper exports hooks so Roadmap 04 can swap in the helper once it lands without refactoring call sites.

## Tests
- Unit tests for the wrapper verifying request payloads and error translation.
- CLI tests ensuring `GRID_ENDPOINT` triggers SDK-based execution with invocation tracking preserved.

## References
- Ploy Workflow RPC Alignment design (`docs/design/workflow-rpc-alignment/README.md`).
- Grid Workflow RPC SDK (`../grid/sdk/workflowrpc/go`).
- Grid Workflow RPC helper guidance (`../grid/sdk/workflowrpc/README.md`).
