# Adopt Grid Workflow RPC Helper

- [x] Completed 2025-10-01

## Why / What For

Once Grid ships the `sdk/workflowrpc/helper` layer, Ploy should migrate its
workflow client construction to use the helper APIs for request building,
configuration, and streaming retries.

## Required Changes

- Replace direct `sdk/workflowrpc/go` usage with helper builders for
  `SubmitRequest` and streaming.
- Update configuration paths to leverage helper environment/token providers.
- Adjust tests to cover helper-based wiring and maintain existing coverage
  thresholds.

## Definition of Done

- Workflow runner and CLI use helper constructors, reducing bespoke client
  wiring.
- Tests validate retryable stream handling and job spec composition via the
  helper.
- Documentation updated to note helper adoption and any required environment
  variables.
- Helper exposes typed HTTP errors so retry logic only triggers on transient
  failures.

## Follow-ups

- 2025-10-05 — Extended the helper-backed client to support Workflow RPC
  cancellation, archive export metadata, and default SDK state directory
  handling. Tracked in `docs/design/workflow-rpc-alignment/README.md` and the
  2025-10-05 changelog entry.

## Tests

- Runner integration test exercising helper-based submission and stream
  handling.
- Unit tests verifying helper configuration errors are surfaced correctly.
- Helper unit tests covering bearer token headers, retry backoff, and context
  cancellation handling.

## References

- Ploy Workflow RPC Alignment design
  (`docs/design/workflow-rpc-alignment/README.md`).
- Grid Workflow RPC helper guide (`../grid/sdk/workflowrpc/README.md`).
- Grid Workflow RPC helper roadmap
  (`../grid/docs/tasks/workflow-rpc/04-sdk-helper-layer.md`).
