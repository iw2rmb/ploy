# Control Plane HTTP Auth Foundation (Roadmap 1.3A)

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 1 of 4 for `docs/next/roadmap.md` item 1.3

## Summary
Introduce a shared authentication + routing layer for the control-plane HTTP server. All forthcoming endpoints will rely on a uniform handler registration helper that wires mutual TLS and bearer-token middleware, request metrics, and structured error responses.

## Goals
- Define and register an auth middleware stack that enforces mutual TLS and bearer tokens for control-plane routes.
- Refactor `internal/api/httpserver/controlplane.go` to route through a helper (`mux.Handle` wrapper) so future endpoints can be added without duplicating boilerplate.
- Expose typed interfaces for downstream handlers to access the authenticated principal and scopes.
- Provide unit tests to ensure middleware rejects unauthenticated requests and propagates context metadata.

## Non-Goals
- Adding mods, artifacts, registry, config, or beacon business logic (handled in later design docs).
- Replacing node-local HTTP surfaces.
- Shipping CLI auth changes (covered in roadmap section 3).

## Current State
- `internal/api/httpserver/controlplane.go` wires handlers directly without auth enforcement.
- Security middleware is not defined; existing helpers only validate HTTP methods and decode JSON.
- Metrics are only registered for `/metrics`.

## Proposed Changes
- Add `internal/api/httpserver/security` providing `Middleware` with mutual TLS verification (via TLS connection state) and JWT validation against `gitlab.Signer`.
- Extend the control-plane server struct to accept an `Auth` dependency; default to a no-op for tests until certs are configured.
- Wrap all existing routes (`/v1/jobs`, `/v1/health`, `/v1/gitlab/...`, `/v1/nodes`, `/v1/beacon/rotate-ca`) with the middleware.
- Introduce helper `registerRoute(mux, method, path, handlerFunc, scopes...)` to simplify future additions.

## Work Plan
1. Implement security middleware package with context principal extraction and scope validation.
2. Update `NewControlPlaneHandler` to require middleware injection and use `registerRoute`.
3. Adapt existing handlers to read auth context where necessary (e.g., GitLab signer admin scope).
4. Cover middleware success/failure cases with unit tests (`internal/api/httpserver/security_test.go`) and regression tests for existing routes.

## Testing Strategy
- Table-driven tests for middleware verifying TLS requirement, token parsing, and scope enforcement.
- Integration-style test using `httptest` server to confirm existing endpoints still respond when authenticated.
- Coverage must keep overall ≥60%.

## Documentation
- Update `docs/next/api.md` auth section if scope names change.
- Document new env vars or config keys in `docs/envs/README.md`.
- Note in implementation review that roadmap 1.3 checkmark occurs after all four design docs are delivered.

## Dependencies
- Requires working etcd and signer configuration from roadmap 1.1/1.2 (already complete).
- Subsequent 1.3 design docs depend on this middleware scaffold.

## COSMIC Sizing

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Control-plane auth routing foundation | 1 | 1 | 1 | 1 | 4 |
| TOTAL | 1 | 1 | 1 | 1 | 4 |
