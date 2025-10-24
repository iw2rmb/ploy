# Control Plane Artifacts & Registry HTTP Surface (Roadmap 1.3C)

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 3 of 4 for `docs/next/roadmap.md` item 1.3
- Depends On: [`docs/design/controlplane-auth-surface/README.md`](../controlplane-auth-surface/README.md), [`docs/design/controlplane-mods-http/README.md`](../controlplane-mods-http/README.md)

## Summary
Deliver the artifact and OCI registry endpoints required by the roadmap. Provide CRUD routes for artifacts plus Docker Registry–compatible manifest and blob operations, building on the control-plane services orchestrated in roadmap 1.4.

## Goals
- Implement `/v1/artifacts/upload`, `/v1/artifacts`, `/v1/artifacts/{cid}`, `/v1/artifacts/{cid}` DELETE with appropriate scope checks.
- Add `/v1/registry/*` routes mirroring Docker Registry v2 semantics (manifest CRUD, blob uploads, tag listing).
- Integrate with `internal/controlplane/registry` and `internal/workflow/artifacts` for persistence, falling back to `501` for write paths until backends land, with error codes documented.
- Stream upload bodies efficiently to avoid memory pressure.

## Non-Goals
- Building artifact storage internals (roadmap item 1.4); this doc focuses on HTTP exposure and placeholders.
- CLI registry tooling (roadmap section 3).
- GC behavior (roadmap section 4).

## Current State
- No artifact or registry HTTP routes exist yet.
- Registry manager partially implemented but hidden behind internal interfaces.
- Artifact metadata storage relies on future IPFS integration.

## Proposed Changes
- Add handler structs for artifacts and registry surfaces using auth helper to enforce scopes (`artifact.read`, `artifact.write`, `registry.push`, etc.).
- Implement read endpoints fully using available metadata; for write operations, structure responses that communicate TODO state (`501` with `error_code`).
- For registry uploads, support chunked streaming to IPFS placeholder, persisting metadata once roadmap 1.4 completes; until then, store upload intents in etcd to unblock CLI validation.
- Register Prometheus metrics for artifact/registry requests and payload sizes.

## Work Plan
1. Define DTOs and auth scopes for artifact and registry operations.
2. Implement read handlers with current metadata sources and tests.
3. Scaffold write handlers to accept payloads, validate requests, and invoke persistence once available; return stubbed `501` while logging TODO.
4. Document responses and integrate metrics.

## Testing Strategy
- Unit tests for list/get endpoints with pagination and filtering.
- Tests ensuring write routes reject when backends unavailable (assert `501` payload).
- Streaming tests for registry uploads verifying body chunking.

## Documentation
- Update `docs/next/api.md` artifact and registry sections to reflect `501` behavior pending roadmap 1.4.
- Record required scopes in `docs/envs/README.md`.
- Reiterate that roadmap 1.3 remains open until design docs 1.3A–1.3D are implemented.

## Dependencies
- Auth & route foundation (1.3A).
- Mods endpoints (1.3B) for shared helpers and DTO patterns.
- Full functionality completes after roadmap item 1.4—this doc establishes HTTP contracts now.

## COSMIC Sizing

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Artifacts and registry HTTP surface | 1 | 1 | 1 | 1 | 4 |
| TOTAL | 1 | 1 | 1 | 1 | 4 |
