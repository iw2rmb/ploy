# Phase 6 — API vs Internal Audit (Codex)

This document captures a focused audit of `api/*` and `internal/*` with an eye toward redundant code, layering violations, and high‑risk architectural decisions. It proposes concrete refactor steps that build on Phases 1–5 and align with the centralized configuration and unified storage direction.

## Executive Summary

- Layering is inverted in multiple places: `internal/*` imports from `api/*`, coupling core to transport and blocking clean reuse/testing.
- Nomad/Consul integrations are duplicated and inconsistent: raw HTTP polling and typed SDK clients coexist with overlapping health logic.
- Configuration has two tracks: `api/config` (file+cache) and `internal/config` (service+sources). These drift and duplicate types/concerns.
- Error handling is inconsistent across HTTP handlers vs `internal/errors` typed errors; response shapes vary by endpoint.
- Small but pervasive duplications (e.g., `getenv` helpers, recipe/catalog models, TLS/routing tags) add surface for drift.

Outcome: consolidate platform logic into `internal/*`, make `api/*` a thin HTTP interface, and standardize on one Nomad/Consul client approach and one configuration service.

## Findings

### 1) Layering Inversion (Critical)

Core packages inside `internal/*` import `api/*` symbols, reversing the intended dependency flow.

- Examples (imports from `internal/*` to `api/*`):
  - `internal/build/*.go` → `github.com/iw2rmb/ploy/api/builders`, `api/envstore`, `api/nomad`, `api/opa`, `api/supply`
- `internal/preview/router.go` → now uses `internal/orchestration` facade (Nomad health + endpoint)
  - `internal/debug/handler.go` → `github.com/iw2rmb/ploy/api/*`
  - `internal/cli/*` → `github.com/iw2rmb/ploy/api/analysis`, `api/arf/*`

Impacts:
- Prevents reuse of core logic outside the HTTP service.
- Increases risk of cycles and accidental behavioral coupling.
- Complicates testability (requires HTTP-layer deps in core tests).

Recommendation:
- Move shared/runtime concerns from `api/*` into `internal/*` (e.g., `envstore`, `nomad`, `opa`, `builders`), expose clean interfaces from `internal/*`, and adjust `api/*` to depend only on those.
- Enforce layering: `api/*` may import `internal/*`; `internal/*` must not import `api/*`.

### 2) Nomad/Consul Duplication and Inconsistency (High)

Two patterns coexist for orchestration and health:

- Raw HTTP polling utilities in `api/nomad/*`:
  - `api/nomad/client.go` (manual HTTP to `/v1/job/...`, custom JSON walk)
  - `api/nomad/health.go` (custom deployment/alloc/task health monitor and consul checks via HTTP)

- SDK-based clients in workflows:
  - `api/analysis/nomad_dispatcher.go` uses `nomad/api` and `consul/api` clients and HCL job templates.
  - `internal/bluegreen/traefik.go` uses `nomad/api` and `consul/api` with service tag manipulation.

Impacts:
- Duplicate health semantics and code paths for job status and allocation health.
- Divergent behavior between subsystems (e.g., timeouts, retry, interpretation of “healthy”).
- Higher maintenance cost; bugs fixed in one path don’t propagate.

Recommendation:
- Standardize on the official SDK clients (`github.com/hashicorp/nomad/api`, `github.com/hashicorp/consul/api`) behind a single `internal/orchestration` facade:
  - Thin `JobService`, `DeploymentService`, `HealthService` interfaces with unified types.
  - One canonical health policy (alloc healthy, task states, readiness).
  - Deprecate raw HTTP helpers in `api/nomad/*` once all call sites are migrated.
- Co-locate Traefik registration and service health in this layer to avoid duplicated tagging/health code.

### 3) Configuration Split-Brain (High)

Two distinct configuration systems create duplication and drift:

- `api/config/config.go`:
  - File-based YAML + in‑process cache and stats; bespoke `StorageConfig`, `ClientConfig`, retry/health types; conversion helpers into `internal/storage` and `factory`.
  - Used by some API handlers (e.g., `api/health/health.go`, storage resolution fallbacks).

- `internal/config/*`:
  - Centralized service with composite loaders (defaults, env, file), hot‑reload, validators, cache.
  - Own `StorageConfig` shape and a `CreateStorageClient()` mapping into `internal/storage/factory`.

Impacts:
- Two `StorageConfig`/retry/cache representations to keep in sync.
- Dual creation paths for storage clients; risk of behavior mismatches.
- Harder to reason about configuration precedence and reload semantics.

Recommendation:
- Make `internal/config.Service` the single source of truth.
- Gradually remove `api/config` by:
  - Moving any missing capabilities (e.g., cache stats, TTL controls) into `internal/config` if truly needed.
  - Updating remaining `api/*` to receive `internal/config.Service` via DI and call `cfg.CreateStorageClient()`.
  - Deleting `CreateStorageClientFromConfig` and `CreateStorageFromFactory` once migration completes and tests pass.

### 4) Error Handling Inconsistency (Medium)

`internal/errors` provides typed, HTTP‑mapped errors, but `api/server` handlers often return ad‑hoc `fiber.Map` with free‑form fields.

- Example: `api/server/handlers.go` returns `fiber.Map{"error": ..., "details": ...}` rather than using a shared error contract.

Impacts:
- Inconsistent response schema across endpoints.
- Harder client integrations; reduced discoverability.

Recommendation:
- Adopt `internal/errors.Error` across `api/server`. Provide middleware to translate domain errors to HTTP consistently:
  - Uniform envelope: `{ "error": { "code": "...", "message": "...", "details": {...} } }`.
  - Replace ad‑hoc error responses in handlers with typed errors and centralized mapping.

### 5) Env Store Placement (Medium)

`api/envstore` implements a file‑backed env store but is imported by core packages:

- `internal/env/*`, `internal/build/*`, tests and helpers import `github.com/iw2rmb/ploy/api/envstore`.

Impacts:
- Another layering inversion; ties core logic to `api/*` location.

Recommendation:
- Move env store to `internal/envstore` (or `internal/env/store`) with a small interface and fs implementation.
- Update imports and keep API handlers depending on the internal interface.

### 6) Routing and Traefik Tagging Duplication (Medium)

Two places manipulate Traefik config via Consul:

- `api/routing/traefik.go`: constructs router/service tags, domain mapping KV, controller/app registration.
- `internal/bluegreen/traefik.go`: rewrites service tags for weighted routing and deployment coloring.

Impacts:
- Tag construction logic is split; risk of divergent naming/semantics.

Recommendation:
- Centralize Traefik tag construction into `internal/routing` with composable helpers. Let blue/green and API registration share the same primitives.
- Keep domain mapping KV read/write in one module with well‑defined schema.

### 7) ARF Model Overlap (Low)

`internal/arf/recipes/models.go` defines `CatalogEntry` while `api/arf/models/*` holds broader ARF models.

Impacts:
- Minor duplication; potential drift on field naming if catalog grows.

Recommendation:
- Either reference `api/arf/models` for shared recipe metadata types or promote common catalog structs into a shared `internal/arf/models` with API importing them.

### 8) Miscellaneous Duplications (Low)

- Repeated `getenv` helpers in multiple packages (`api/nomad/client.go`, `internal/preview/router.go`).
- Multiple storage retry/cache config type definitions across `api/config` and `internal/storage`/`internal/config`.
- Legacy/deprecated endpoints retained alongside new flows (`internal/cert/handler.go` vs `api/acme/*`).

Recommendation:
- Introduce `internal/utils/env.go` for tiny helpers; dedupe call sites.
- Keep a single set of retry/cache config types (source: `internal/config` + `internal/storage/factory`).
- Remove deprecated cert endpoints once clients migrate; keep a timeline and feature flag if needed.

## Proposed Refactor Plan

1) Uninvert Dependencies (Phase 6.1)
- Extract shared logic from `api/*` to `internal/*`:
  - Move `api/envstore` → `internal/envstore` with same interface.
  - Move Nomad health utilities behind `internal/orchestration` facade; re‑export minimal interfaces.
- Update `internal/*` imports to target internal packages only.
- Add a static check in CI to forbid `internal/*` importing `api/*`.

2) Orchestration Unification (Phase 6.2)
- Create `internal/orchestration` with Nomad/Consul SDK clients and a single health policy:
  - Services: `Jobs`, `Deployments`, `Health`, `ServiceRegistry`.
  - Migrate: `api/nomad/*` and raw HTTP → the unified facade.
  - Update `internal/bluegreen` and `api/analysis` to depend on the same facade.

3) Configuration Convergence (Phase 6.3)
- Make `internal/config.Service` mandatory for API server (already trending via Phases 2–3).
- Replace remaining uses of `api/config` with DI of `internal/config.Service`:
  - Delete `CreateStorageClientFromConfig` and `CreateStorageFromFactory` after migration.
  - Keep a compatibility shim only in tests if necessary.

4) Error Contract Standardization (Phase 6.4)
- [x] Add an HTTP error middleware mapping `internal/errors.Error` → JSON envelope.
- [x] Refactor `api/server/*` handlers to return typed errors; reduce ad‑hoc `fiber.Map` error payloads.
- [x] Add unit tests asserting shape and codes.

5) Routing Consolidation (Phase 6.5)
- Extract Traefik tag generation to `internal/routing/tags.go`.
- Make domain mapping KV schema/versioned and documented.
- Use the same helpers from blue/green and controller/app registration.

6) Cleanup and Dedupe (Phase 6.6)
- [x] Introduce shared env utilities in `internal/utils` (e.g., `Getenv`).
- [x] Replace scattered inline `getenv` usages (preview/orchestration/cleanup) with shared helper.
  - Implemented in `internal/orchestration/render.go` and `internal/cleanup/config.go`; preview already used helpers.
- [x] Remove deprecated cert endpoints under `internal/cert/*` after migration window.
  - Deleted `internal/cert/handler.go` (legacy stubs). No active routes referenced it.
- [ ] Unify small config types (retry/cache) on the `internal/config` + `internal/storage/factory` axis.

## Prioritization (Risk x Blast Radius)

1. Layering inversion fix (break cycles, unblock reuse/testing).
2. Orchestration unification (eliminate split health semantics and polling code).
3. Config convergence (remove split‑brain and duplicate types).
4. Error contract standardization (client experience, observability).
5. Routing consolidation (consistency, fewer bugs in traffic shifting).
6. Misc cleanup (low risk, steady paydown).

## Concrete Call‑Site Inventory (Non‑exhaustive)

- Internal → API imports to migrate:
  - `internal/build/{trigger.go,status.go,logs.go}` → `api/nomad`, `api/opa`, `api/envstore`, `api/builders`.
  - `internal/preview/router.go` → `api/nomad`.
  - `internal/debug/handler.go` → `api/*`.
  - `internal/cli/arf/*`, `internal/cli/analysis/*` → `api/arf/*`, `api/analysis`.

Progress (Sep 4):
- [x] internal/cli/arf switched to `internal/arf/models` (partial — templates still reference API models).

- Raw HTTP Nomad utilities to remove after unification:
  - `api/nomad/client.go`, `api/nomad/health.go`.

- Config dual paths to remove:
  - `api/config/config.go`: `CreateStorageClientFromConfig`, `CreateStorageFromFactory` and bespoke config types.

## Acceptance Criteria

- [x] No `internal/*` package imports any path under `api/*`.
  - Replaced facades with internal implementations:
    - `internal/builders/facade.go` no longer imports `api/builders`.
    - `internal/supply/facade.go` no longer imports `api/supply`.
  - Added guardrail test `internal/no_api_imports_test.go`.
- [x] All Nomad/Consul operations go through `internal/orchestration` with SDK clients.
  - Health monitor now uses Nomad SDK via injectable adapter; HTTP calls removed.
  - Consul health checks now use Consul SDK via injectable adapter; HTTP calls removed.
  - Added constructor for injection in tests (`NewHealthMonitorWithClient`).
- [ ] API server and CLI resolve storage via `internal/config.Service` only.
- [x] HTTP errors use a single envelope with typed codes; tests assert shape.
- [ ] Traefik tags and domain KV are generated via shared helpers.
  - Tags now centralized via `internal/routing/BuildTraefikTags` and used by API routing. Domain KV helpers pending.

## Progress (Sep 4, 2025)

- [x] Preview router migrated to `internal/orchestration` facade (no `api/nomad` dependency).
- [x] Standardized HTTP error envelope in API server via `internal/errors` with unit tests (`api/server/error_handler_test.go`).
- [ ] `internal/*` still imports `api/*` in a few packages (`internal/debug`, `internal/build`).
- [x] Raw HTTP helpers deprecated in `api/nomad/client.go` by delegating to `internal/orchestration` monitor.
- [ ] Traefik tag builders remain in `api/routing`; shared helpers under `internal/routing/*` not yet extracted.

## Notes and Constraints

- VPS and deployment flows must continue to use the platform’s Nomad invocation rules; do not introduce direct Nomad CLI usage.
- API deployment lanes remain auto‑selected; ensure the refactor keeps the Lane G (WASM) rules intact.
- Keep unit tests humming locally (≥60% coverage; 90% for critical paths), then exercise E2E on VPS per CLAUDE.md.
