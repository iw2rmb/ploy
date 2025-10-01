# Discovery Configuration Tests
- [x] Completed (2025-09-29)

## Why / What For
The current tests only assert that discovery populates JetStream/IPFS strings, missing coverage for the new API endpoint, feature map, multi-route handling, and cache behaviour. Extending the tests ensures the CLI stays aligned with Grid's guarantees and guards against regressions when the payload evolves.

## Required Changes
- Update `cmd/ploy/dependencies_discovery_test.go` to validate that the parser captures API endpoint, version, and features while preferring discovery data over fallbacks.
- Add table-driven cases covering multi-route JetStream lists, disabled features (`""` values), cache reuse, and JSON validation failures.
- Introduce helper assertions for feature checks to mirror the behaviour exported by `integrationConfig`.

## Definition of Done
- Tests fail (RED) against the current implementation due to missing fields/behaviour.
- After implementation, tests pass and exercise discovery success, fallback, and cache scenarios.
- Coverage includes feature gate propagation and ensures the cache avoids duplicate network calls.

## Tests to Perform
- `go test ./cmd/ploy -run TestResolveIntegrationConfigUsesDiscovery` (expanded assertions).
- `go test ./cmd/ploy -run TestResolveIntegrationConfigFallsBackToEnv`.
- New table-driven tests (e.g., `TestIntegrationConfigFeatureGates`) verifying helper behaviour.

## Status Log
- 2025-09-29 — RED: Expanded discovery tests to assert API endpoint, version, features, and caching; `go test ./cmd/ploy` failed until parser updates landed.
- 2025-09-29 — GREEN: Tests passing with new schema coverage and feature helper assertions; `go test ./cmd/ploy` successful.

## Links & References
- Design: `../../docs/design/discovery-alignment/README.md`.
- Grid references: `../../../../grid/docs/api/openapi.yaml`, `../../../../grid/sdk/discovery/go/client.go`.
