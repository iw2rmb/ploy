# Discovery Payload Parser Expansion

- [x] Completed (2025-09-29)

## Why / What For

Grid's discovery API now serves `ClusterInfoResponse` with API endpoint,
JetStream seed list, IPFS gateway, feature gates, and version (see
`../../../../grid/docs/api/openapi.yaml`). Ploy still models the payload as a
single JetStream URL plus IPFS gateway, so it discards feature data and ignores
additional routes that operators rely on for HA. Updating the parser keeps
workstation behaviour in lockstep with the control plane and unlocks
feature-aware tooling.

## Required Changes

- Replace the ad-hoc struct in `cmd/ploy/dependencies.go` with one matching the
  OpenAPI schema (api endpoint, jetstream_urls slice, ipfs gateway, features
  map, version string).
- Extend `integrationConfig` to expose the additional fields (API endpoint,
  version, features) and rely exclusively on discovery responses when discovery
  is reachable.
- Prefer the first non-empty JetStream URL for existing clients while retaining
  the full slice for future consumers.
- Enforce strict JSON decoding (`DisallowUnknownFields`) and clear cache entries
  when payloads fail validation.
- Provide helper methods to query feature gates (case-insensitive `enabled`
  comparison) without duplicating string comparisons across call sites.

## Definition of Done

- `cmd/ploy/dependencies.go` compiles with the new schema, exposes the feature
  map, and returns discovery-backed configuration when available.
- When discovery is unreachable, callers receive sanitized defaults without
  JetStream/IPFS data while the in-memory stubs stay active.
- Helper enables callers to evaluate feature states without re-fetching
  discovery.
- Unit tests cover success, fallback, disabled feature, and cache scenarios (see
  Task 02).

## Tests to Perform

- `go test ./cmd/ploy -run TestResolveIntegrationConfigUsesDiscovery` (updated
  to assert full payload handling).
- `go test ./cmd/ploy -run TestResolveIntegrationConfigReturnsStubWhenDiscoveryFails`
  (ensuring failure cases surface sanitized defaults).
- New tests covering feature checks and multi-route support (coordinated with
  Task 02).

## Status Log

- 2025-09-29 — RED: Added discovery config tests exercising multi-route handling
  and feature map exposure; `go test ./cmd/ploy` failed as expected.
- 2025-09-29 — GREEN: Updated parser/cache helpers, removed legacy environment
  fallbacks, and confirmed `go test ./cmd/ploy` passes.

## Links & References

- Design: `../../docs/design/discovery-alignment/README.md`.
- Grid references: `../../../../grid/docs/api/openapi.yaml`,
  `../../../../grid/sdk/discovery/go`.
