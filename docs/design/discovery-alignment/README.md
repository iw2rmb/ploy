# Grid Discovery Alignment

## Purpose
Ensure Ploy mirrors Grid's `/v1/cluster/info` surface so workstation workflows consume the authoritative discovery payload (JetStream routes, IPFS gateway, API endpoint, feature gates, version) without relying on stale assumptions.

## Status
- [x] [roadmap/discovery-alignment/01-cluster-info-parser.md](../../../roadmap/discovery-alignment/01-cluster-info-parser.md) — Update `cmd/ploy/dependencies.go` to parse the full discovery schema, expose features, and respect multi-URL JetStream responses.
- [x] [roadmap/discovery-alignment/02-discovery-config-tests.md](../../../roadmap/discovery-alignment/02-discovery-config-tests.md) — Expand discovery configuration tests to cover new fields, feature gating, and cache behaviour.
- [x] [roadmap/discovery-alignment/03-workflow-grid-alignment.md](../../../roadmap/discovery-alignment/03-workflow-grid-alignment.md) — Align workflow runner Grid tests with the expanded discovery integration.

Status as of 2025-09-29: Completed — discovery parser, feature surfacing, tests, docs, and roadmap entries updated.

## Background
Ploy currently expects discovery to return a single JetStream URL and IPFS gateway (`cmd/ploy/dependencies.go`), ignoring the API endpoint, semantic version, and feature map codified in Grid's contract. The Grid control plane shipped an expanded payload on 2025-09-29, documented in the OpenAPI spec and design narrative. Without alignment, workstation users miss feature gate signals (e.g., `scheduler_control`) and fall back to legacy environment variables even when discovery is authoritative. Inspection on 2025-09-29 of `cmd/ploy/dependencies.go` and `cmd/ploy/dependencies_discovery_test.go` confirmed the mismatch.

## Goals
- Parse and cache the full `ClusterInfoResponse`, including API endpoint, JetStream URL list, IPFS gateway, feature map, and version.
- Prefer discovery-provided JetStream routes/IPFS gateway while returning sanitized defaults (and relying on in-memory stubs) when discovery is unavailable or feature gates disable an integration.
- Surface feature gate helpers so future slices (scheduler control, workspace API) can toggle behaviour without re-querying discovery.
- Keep retry and caching logic intact while tightening payload validation (unknown fields, empty responses).

## Non-Goals
- Implement new CLI commands for feature-gated workflows (covered by future slices).
- Add network retries beyond the existing backoff window.
- Alter authentication flows or token sourcing for discovery requests.

## Proposed Changes
1. Extend `clusterInfo` parsing to reflect Grid's OpenAPI schema, store JetStream routes as a slice, capture API endpoint/version, and track the feature map (Task 01).
2. Enhance `resolveIntegrationConfig` to prefer discovery data, surface the discovery feature map for downstream gating, and remove legacy environment fallbacks while keeping explicit error propagation (Task 01).
3. Broaden CLI discovery tests to assert multi-route handling, feature map exposure, cache hits, and sanitized-default semantics when discovery fails (Task 02).
4. Update workflow Grid integration tests to assert discovery bootstrap succeeds with the richer payload and to exercise feature-gated fallbacks (Task 03).
5. Document behaviour changes and record verification details in the changelog and roadmap once GREEN (Tasks 01-03 conclude).

## Dependencies
- [`../../../../grid/docs/api/openapi.yaml`](../../../../grid/docs/api/openapi.yaml) — Authoritative schema for `ClusterInfoResponse` and `/v1/cluster/info`.
- [`../../../../grid/docs/design/api/README.md#cluster-discovery-shipped-september-29-2025`](../../../../grid/docs/design/api/README.md#cluster-discovery-shipped-september-29-2025) — Narrative contract covering auth, feature gates, and runtime behaviour.
- [`../../../../grid/docs/design/deploy/README.md#http-discovery-surface`](../../../../grid/docs/design/deploy/README.md#http-discovery-surface) — Operational validation guidance (curl workflow) ensuring payload fidelity.
- [`../../../../grid/sdk/discovery/go`](../../../../grid/sdk/discovery/go) and [`../../../../grid/sdk/discovery/ts`](../../../../grid/sdk/discovery/ts) — Reference clients demonstrating helper expectations (`FeatureEnabled`, multi-URL handling).
- [`../../../../grid/CHANGELOG.md`](../../../../grid/CHANGELOG.md) — Release note capturing delivery date and verification command (`go test ./internal/httpapi ./sdk/discovery/go`).

## Risks & Mitigations
- **Schema drift**: Adopt strict decoding (`json.Decoder.DisallowUnknownFields`) and align tests with the OpenAPI fixture.
- **Feature gate misuse**: Provide helper functions/tests ensuring absent or disabled features default safely to legacy behaviour.
- **Breaking cached payloads**: Version the cache entries by endpoint and ensure stale entries refresh when discovery fails validation.

## Test Strategy
- Unit tests in `cmd/ploy/dependencies_discovery_test.go` covering success, sanitized-default, disabled-feature scenarios, and caching.
- CLI workflow tests in `cmd/ploy/workflow_run_grid_test.go` to confirm Grid runs bootstrap correctly with discovery present/absent.
- `go test ./cmd/ploy` as the primary verification command; additional targeted packages unchanged.

## Deliverables
- Updated discovery client and configuration wiring in `cmd/ploy/dependencies.go` handling the full payload.
- Extended unit tests reflecting the new schema, feature gates, and sanitized-default handling when discovery is unavailable.
- Documentation and changelog entries describing the behaviour and verification steps.

## Verification Plan
- 2025-09-29 — Confirmed current mismatch by inspecting `cmd/ploy/dependencies.go` and `cmd/ploy/dependencies_discovery_test.go`; recorded here for traceability.
- 2025-09-29 — `go test ./cmd/ploy` after implementation, confirming new parser + feature map plumbing against `cmd/ploy/dependencies.go`, `cmd/ploy/dependencies_discovery_test.go`, and `cmd/ploy/workflow_run_grid_test.go`; results captured in roadmap status logs.
