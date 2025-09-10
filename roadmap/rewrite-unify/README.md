# Migration Plan: Remove ARF `transforms` HTTP, Unify on `transflow`

## Overview

Collapse the dual surface into a single API: remove ARF transform endpoints entirely and use only Transflow for all transformation workflows.

- Current:
  - ARF transforms: `/v1/arf/transforms/*` (async single-recipe + debug/reporting)
  - Transflow: `/v1/transflow/*` (orchestrated workflows, artifacts, logs, events)
- Target:
  - Only `/v1/transflow/*` for all transformation operations
  - Hard removal of `/v1/arf/transforms*` and related code (no deprecation)

Backwards compatibility is NOT required. Remove legacy surfaces and references in one change set, and update docs/tests accordingly.

## Pitfalls Identified (must address)

- Mixed singular/plural and legacy paths in code/docs/tests:
  - Legacy singular: `/v1/arf/transform` appears in CLI/docs/comments; no server route exists.
  - ARF plural: `/v1/arf/transforms/*` still registered and used across tests/scripts.
  - Transflow docs mix `/v1/transflow/*` and `/v1/transflows/*`; only singular is implemented.
- CLI “ploy arf transform” still posts to `/arf/transform` and will break when ARF transforms are removed. No compatibility mode: remove this command and its docs.
- Test scripts and unit tests reference ARF transforms; expand sweep beyond the few listed earlier.
- Consul KV prefix uses `ploy/arf/transforms` in tests; decide and update naming to `ploy/transflow/*` or keep legacy prefix intentionally. No runtime data migration is required.
- ARF debug/report endpoints (hierarchy/timeline/analysis/report/orphaned) will disappear; explicitly remove and update docs. Optional rehome under transflow can be considered later as a new feature.
- Orphan file: `api/arf/transformation_workflow.go` appears unused; safe to remove.

## Detailed Migration Plan (hard removal)

### Phase 1: Remove ARF Transform HTTP Surface

Code changes:
- `api/arf/handler.go`
  - Remove route registrations for:
    - `POST /v1/arf/transforms`
    - `GET  /v1/arf/transforms/:id`
    - `GET  /v1/arf/transforms/:id/status`
    - `GET  /v1/arf/transforms/:id/{hierarchy|active|timeline|analysis|report}`
    - `GET  /v1/arf/transforms/orphaned`
- Delete transform handlers and debug/report endpoints:
  - `api/arf/handler_transformation_async.go`
  - `api/arf/transformation_internal.go`
  - Remove transform-specific endpoints in `api/arf/handler_debug.go` (hierarchy/active/timeline/analysis/report/orphaned).
- Remove unused/legacy workflow file:
  - `api/arf/transformation_workflow.go` (not referenced; safe to delete or archive under roadmap if you prefer).

Notes:
- Keep ARF internals used by Transflow (OpenRewrite engine/dispatcher, healing coordinator types, recipe registry, SBOM/security, sandbox interfaces).

### Phase 2: CLI and Help Surface

- Remove the “ploy arf transform” command and help/examples:
  - Delete `internal/cli/arf/transform.go` and strip references from `cmd/ploy/README.md` and any ARF help files.
- Ensure Transflow is the sole CLI for workflows:
  - Continue to promote `ploy transflow run` and the Transflow config path.
- Remove unused integration shims:
  - In `internal/cli/transflow/integrations.go`, remove `ARFRecipeExecutor` (wrapper that shells out to `ploy arf transform`), and any calls to it. The current Transflow runner does not depend on it; ensure no dead references remain.

### Phase 3: Tests and Scripts

- Update/remove all direct ARF transform references in scripts:
  - `tests/scripts/test-arf-phase2.sh` (remove POST/GET to `/v1/arf/transforms*`).
  - `tests/scripts/test-transformation-workflow.sh`.
  - `tests/scripts/test-arf-unified-consistency.sh` (has references to `/arf/transform`).
  - `tests/scripts/test-openrewrite-comprehensive.sh`.
  - `tests/scripts/test-storage-fix-verification.sh`.
- Update unit tests that encode the Consul key prefix `ploy/arf/transforms` if we rename it (see Phase 5).

Recommended repo-sweep to catch stragglers:
```
rg -n '/v1/arf/transforms|/v1/arf/transform\b' 
rg -n '/v1/transflows\b'  # fix pluralization in docs
```

### Phase 4: Documentation Sweep (single Transflow surface)

- Remove ARF transform references and update examples end-to-end:
  - `api/arf/README.md` – drop transforms section and examples.
  - `docs/ARF_OPENREWRITE_MIGRATION_GUIDE.md` – remove “/arf/transform(s)” calls; update migration guidance to point at Transflow.
  - `docs/recipes.md` – remove POST `/v1/arf/transforms` and related status examples.
  - `README.md` and any root or product docs with transform examples.
  - `roadmap/transformations/README.md` – remove “/v1/arf/transforms” timeline/status samples.
- Normalize Transflow docs and remove unimplemented endpoints:
  - `docs/api/transflow.md`
    - Ensure only implemented endpoints are documented:
      - `POST   /v1/transflow/run`
      - `GET    /v1/transflow/status/:id`
      - `GET    /v1/transflow/list`
      - `DELETE /v1/transflow/:id`
      - `GET    /v1/transflow/artifacts/:id[/:name]`
      - `POST   /v1/transflow/event`
      - `GET    /v1/transflow/logs/:id`
    - Remove plural “/v1/transflows/*” variants and any “config/template|config/validate” sections if not implemented.
- Update `CHANGELOG.md` and `docs/FEATURES.md` per protocol to record the breaking change (removal) and the unified API surface.

### Phase 5: Consul KV Prefix (optional but recommended)

- Rename Consul key prefix from `ploy/arf/transforms` to a Transflow‑scoped prefix, e.g., `ploy/transflow/status`.
- Update store initialization and tests referencing the old prefix (e.g., `api/arf/consul_store_test.go`).
- No data migration required (stateless by contract); document the new prefix in developer docs.

### Phase 6: Nomad Templates and Runners

- No changes required for runner templates that emit `TRANSFORMATION_ID` or use transformation‑centric naming inside jobs; these are internal to jobs and orthogonal to HTTP removal.
- Verify that Transflow jobs and services (e.g., `services/openrewrite-jvm/runner.sh`) keep reporting to `/v1/transflow/event` and persist artifacts under Transflow paths.

### Phase 7: Build, Format, Static Analysis, and Validation

- Build: `go build ./...`
- Static analysis: `staticcheck ./...`
- Formatting: `goimports -w .` and `gofmt -s -w .` (for any Go edits in the removal PR)
- Tests: `make test-unit` and `make test-coverage-threshold`
- Validation:
  - Ensure `rg` sweeps above return 0 hits for legacy ARF transform paths in non‑roadmap docs.
  - Transflow handler tests (`api/transflow/handler_test.go`) pass.
  - Transflow E2E/integration tests remain green in VPS (REFACTOR phase).

## Keep vs Remove

Keep (Transflow dependencies):
- `api/arf/openrewrite_engine.go`, `api/arf/openrewrite_dispatcher.go`, `api/arf/engine.go`, `api/arf/factory.go`.
- Consul status/healing types: `api/arf/consul_types.go` and healing coordinator components.
- Recipe registry and SBOM/security endpoints.

Remove (legacy ARF transform surface):
- All HTTP endpoints under `/v1/arf/transforms/*` and their handlers.
- CLI “ploy arf transform” command and docs/examples.
- Transform‑only scripts/tests/docs and any leftover “/v1/arf/transform” singular references.
- Unused `api/arf/transformation_workflow.go`.

## Success Criteria

1. Only `/v1/transflow/*` endpoints exist (singular path), documented and tested.
2. No references to `/v1/arf/transforms` or `/v1/arf/transform` remain in code, tests, or product docs.
3. All unit tests pass and staticcheck is clean after removals; coverage thresholds maintained.
4. E2E transflow workflows function as before (artifacts, events, logs, status).
5. CHANGELOG and FEATURES updated to reflect the hard removal.

## Notes

- No deprecation window; removal is immediate.
- No data migration; ARF transform endpoints were stateless, and Transflow maintains its own status/artifacts.
- Optional future work: rehome selected ARF debug/report visualizations under Transflow if needed by users.

## Quick Reference: Files/Areas to Update

- Routes/Handlers:
  - `api/arf/handler.go`, `api/arf/handler_transformation_async.go`, `api/arf/transformation_internal.go`, `api/arf/handler_debug.go` (transform endpoints only)
  - remove `api/arf/transformation_workflow.go`
- CLI:
  - remove `internal/cli/arf/transform.go`, update `cmd/ploy/README.md` and ARF help files
  - clean `internal/cli/transflow/integrations.go` (remove `ARFRecipeExecutor`)
- Tests/Scripts:
  - `tests/scripts/test-arf-phase2.sh`, `tests/scripts/test-transformation-workflow.sh`, `tests/scripts/test-arf-unified-consistency.sh`,
    `tests/scripts/test-openrewrite-comprehensive.sh`, `tests/scripts/test-storage-fix-verification.sh`
  - unit tests referencing `ploy/arf/transforms` prefix if we rename
- Docs:
  - `api/arf/README.md`, `docs/recipes.md`, `docs/ARF_OPENREWRITE_MIGRATION_GUIDE.md`, `docs/api/transflow.md`, root `README.md`, `roadmap/transformations/README.md`
  - `CHANGELOG.md`, `docs/FEATURES.md`
