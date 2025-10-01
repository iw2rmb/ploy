# Workflow Grid Alignment for Discovery
- [x] Completed (2025-09-29)

## Why / What For
`handleWorkflowRun` relies on discovery to configure JetStream and snapshots when `GRID_ENDPOINT` is set. The tests currently stub discovery with minimal payloads, so they do not cover feature gates or the richer schema. Updating the tests guarantees the workflow runner continues to operate when Grid exposes additional discovery data or disables integrations via feature flags.

## Required Changes
- Adjust `cmd/ploy/workflow_run_grid_test.go` fixtures to provide full discovery payloads matching Grid's contract.
- Add assertions that the workflow runner respects discovery-derived configuration while tolerating disabled features (e.g., missing snapshot manager).
- Ensure tests cover both discovery success and failure paths with the updated helpers from Task 01.

## Definition of Done
- Workflow Grid tests fail (RED) until the discovery parser exposes the new fields and feature helpers.
- Post-implementation, tests pass using the richer payload and verify correct behaviour when discovery is unavailable or returns disabled features.
- Test suite documents expectations for future roadmap slices needing feature gates.

## Tests to Perform
- `go test ./cmd/ploy -run TestHandleWorkflowRunUsesGridEndpointClient`.
- `go test ./cmd/ploy -run TestHandleWorkflowRunFailsForInvalidGridEndpoint`.
- Additional tests if needed for feature-gated paths.

## Status Log
- 2025-09-29 — RED: Updated workflow Grid tests to assert discovery payload contents; `go test ./cmd/ploy` failed until parser surfaced new fields.
- 2025-09-29 — GREEN: Workflow tests passing with discovery assertions and caching; `go test ./cmd/ploy` successful.

## Links & References
- Design: `../../docs/design/discovery-alignment/README.md`.
- Grid references: `../../../../grid/sdk/discovery/go`, `../../../../grid/docs/design/api/README.md#cluster-discovery-shipped-september-29-2025`.
