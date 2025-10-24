# Control Plane Config, Status & Beacon HTTP Surface (Roadmap 1.3D)

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 4 of 4 for `docs/next/roadmap.md` item 1.3
- Depends On: 
  - [`docs/design/controlplane-auth-surface/README.md`](../controlplane-auth-surface/README.md)
  - [`docs/design/controlplane-mods-http/README.md`](../controlplane-mods-http/README.md)
  - [`docs/design/controlplane-artifacts-registry/README.md`](../controlplane-artifacts-registry/README.md)

## Summary
Expose the remaining control-plane routes for cluster configuration, status, version reporting, and beacon discovery. This slice completes roadmap item 1.3 and enables roadmap sections 3–5 to consume the refreshed surfaces.

## Goals
- Implement `/v1/config` GET/PUT backed by `internal/api/config` with scope enforcement and optimistic concurrency.
- Serve `/v1/status` and `/v1/version` aggregating scheduler metrics and deployment metadata.
- Provide `/v1/beacon/*` read endpoints (`/v1/beacon/nodes`, `/v1/beacon/ca`, `/v1/beacon/config`, `/v1/beacon/promote`) alongside existing rotate-CA route.
- Ensure responses align with `docs/next/api.md` schema and include caching headers where appropriate.
- Document completion criteria to checkmark roadmap item 1.3 and commit once combined work from 1.3A–1.3D ships.

## Non-Goals
- Implementing CA rotation internals (already covered by existing handler).
- Worker/node status APIs (roadmap section 2).
- CLI command updates (roadmap section 3).

## Current State
- Only `/v1/health`, `/v1/gitlab/*`, `/v1/nodes`, and `/v1/beacon/rotate-ca` exist.
- Config package provides etcd-backed storage but lacks HTTP access.
- Beacon discovery helpers exist under `deploy` yet no HTTP wrappers.

## Proposed Changes
- Add handlers accessing etcd for configuration with ETag-style versioning to avoid write clobber.
- Aggregate scheduler/node metrics for `/v1/status` and return semantic version string for `/v1/version`.
- Implement beacon read endpoints using deploy/beacon helpers, verifying responses are signed where required.
- Register additional Prometheus counters for config writes and beacon requests.

## Work Plan
1. Implement `/v1/config` GET/PUT with validation, version precondition headers, and tests.
2. Build `/v1/status` aggregator summarizing queue depth, nodes, Mods throughput using existing metrics registries.
3. Implement `/v1/version` returning build metadata (commit, semver).
4. Add beacon read endpoints, reusing existing rotation logic for signing.
5. Update docs and, after verifying all slices (1.3A–1.3D) are implemented, checkmark `docs/next/roadmap.md` item 1.3 and commit.

## Testing Strategy
- Unit tests for config read/write covering optimistic concurrency.
- Tests for status/version verifying payload shape via golden files.
- Beacon endpoint tests mocking deploy helpers and ensuring auth scope requirements.

## Documentation
- Sync `docs/next/api.md` with final payload examples.
- Update `docs/next/devops.md` and `docs/next/vps-lab.md` with new endpoints for operators.
- Explicitly state in implementation PR/commit message that roadmap 1.3 is checkmarked when all four design docs are complete.

## Dependencies
- Requires middleware foundation (1.3A) and route scaffolding introduced earlier slices.
- Consumes metrics populated by prior roadmap work (1.1–1.2).
- Last slice needed before marking roadmap item 1.3 complete.

## COSMIC Sizing

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Config, status, version, beacon HTTP surface | 1 | 1 | 1 | 1 | 4 |
| TOTAL | 1 | 1 | 1 | 1 | 4 |
