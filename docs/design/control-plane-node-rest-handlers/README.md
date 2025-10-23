# Control-Plane Node REST Handlers (Phase 1)

## Problem

`ployd` control-plane tasks that manage worker onboarding, CA rotation, and GitLab signer updates
still invoke etcd directly. Operators must tunnel etcd and export `PLOY_ETCD_ENDPOINTS`, despite the
v2 API outlining `/v2/nodes`, `/v2/beacon/rotate-ca`, and `/v2/config/gitlab`. This gap blocks the
Ploy v2 fundamental goal of delegating privileged workflows to authenticated HTTPS endpoints.

## Goals

- Serve `POST /v2/nodes`, `GET /v2/nodes`, and `DELETE /v2/nodes` from the control-plane `ployd`
  deployment, delegating persistence to existing deploy packages.
- Wrap CA rotation with `POST /v2/beacon/rotate-ca`, mirroring the CLI’s current etcd sequence.
- Expose `GET /v2/config/gitlab` and `PUT /v2/config/gitlab` so the CLI no longer loads GitLab
  signer secrets from etcd.
- Maintain compatibility with bootstrap descriptors (CA bundle, API key, beacon URL) for mutual TLS
  + bearer token auth.

## Scope

- Control-plane HTTP handlers, request/response validation, and integration with
  `deploy.RunWorkerJoin`, CA managers, and GitLab config registry helpers.
- Common auth middleware that enforces mutual TLS, bearer token scopes, and cluster ID matching.
- Minimal CLI stubs behind a feature flag solely to exercise the new endpoints in unit/integration
  tests (full CLI migration is a later slice).

## Non-Goals

- Refactoring the worker registry schema or etcd layout.
- Removing the legacy CLI code paths; they remain until the CLI slice flips the feature flag.
- Telemetry and dashboard workstreams.

## Implementation Outline

1. **Transport**  
   - Extend the control-plane router with `/v2/nodes`, `/v2/beacon/rotate-ca`, `/v2/config/gitlab`.  
   - Share TLS + token auth middleware that extracts descriptor metadata.
2. **Handlers**  
   - `POST /v2/nodes`: validate payload, call `deploy.RunWorkerJoin`, return worker ID, cert bundle,
     probe summary, and generated credentials.  
   - `GET /v2/nodes`: reuse registry listing, including health, labels, workload counters.  
   - `DELETE /v2/nodes`: delegate to existing drain/deregister helper, require confirmation token.  
   - `POST /v2/beacon/rotate-ca`: invoke CA rotation manager, returning new bundle metadata.  
   - `GET/PUT /v2/config/gitlab`: wrap GitLab signer config storage with optimistic locking.
3. **Testing**  
   - Unit tests with `httptest.Server` + in-memory fakes for registry, CA manager, GitLab config.  
   - Integration test: seeded etcd, invoke `POST /v2/nodes`, assert worker entry, cert issuance, and
     cleanup through `DELETE`.
4. **Docs & Telemetry Hooks**  
   - Update `docs/v2/api.md` if response schemas diverge.  
   - Emit structured logs for audit of onboarding and CA rotations.

## Open Questions

- None.

## Requirements

- Node deregistration endpoints MUST block until all running jobs complete before removing the node.
  This guarantees parity with today’s CLI drain expectations and prevents orphaned workloads when
  operators remove nodes via the REST path.

## Exit Criteria

- Endpoints respond with parity to existing CLI workflows without etcd tunnelling.
- CLI smoke harness (feature flag on) can add a worker and rotate CA via HTTPS calls.
- Documentation in `docs/design/QUEUE.md` reflects completion readiness for CLI follow-up work.
