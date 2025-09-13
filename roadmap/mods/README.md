# Mods Rename + RESTful API/CLI Plan

Goal

- Rename the feature set currently named "Transflow" to "Mods" across code, tests, docs, and platform assets.
- Align HTTP endpoints to RESTful conventions (resource-oriented paths), e.g. `transflow/status/:id` → `mods/:id/status`.
- Change CLI subcommands from `ploy mod ...` to `ploy mod ...`.

Scope (What changes)

- Code: packages, symbols, filenames, constants, env keys (where user-facing), storage keys, job templates/paths, events.
- API: routes, handlers, OpenAPI/docs, server-side event endpoints, artifacts paths.
- CLI: command group, flags, help, usage text, examples.
- Docs: README, FEATURES, API docs, examples, roadmap references.
- Tests: unit, integration, E2E (rename fixtures, assertions, golden files, URLs, storage keys).

Constraints

- Obey AGENTS.md (no raw Nomad, wrapper usage preserved). Do not change runtime behavior beyond naming/paths.
- Backward compatibility is NOT required. Remove legacy code and artifacts.

High-level Phases

1) Rename API, CLI, code, and storage to Mods (single cutover)
2) Delete legacy Transflow endpoints, code, and artifacts
3) Update docs and examples

Detailed Work Breakdown

0. Prep and utilities
- Prepare rg/sed scripts to perform atomic renames and protect unrelated matches (boundaries/paths only).

1. API routes (server)
- New resource: `mods` representing a single run (replaces `transflow`).
- Implement only the new RESTful endpoints (no legacy routes):
  - POST `/v1/mods` — create/run; returns `{ id, status_url, logs_url, artifacts_url }`
  - GET `/v1/mods/:id` — get run (includes core fields + last status)
  - GET `/v1/mods/:id/status` — latest status snapshot
  - GET `/v1/mods` — list runs
  - DELETE `/v1/mods/:id` — cancel run
  - GET `/v1/mods/:id/artifacts` — list artifacts
  - GET `/v1/mods/:id/artifacts/:name` — fetch single artifact
  - POST `/v1/mods/:id/events` — push events
  - GET `/v1/mods/:id/logs` — server-sent events or polling (`follow` query supported)
- Update handler wiring and tests (api/transflow → api/mods), add route group `/v1/mods`.
- Update artifact key policy to `artifacts/mods/:id/...` (no dual-read required).

2. CLI rename
- Add `ploy mod` command group (`run`, `watch`, `status`, etc.).
- Remove `ploy mod` group entirely.
- Update CLI help/README and examples.

3. Mechanical rename in codebase
- Packages/paths:
  - `internal/mods` → `internal/mods`
  - `api/transflow` → `api/mods`
  - `docs/mods` → `docs/mods`
  - `platform/nomad/transflow/*` → `platform/nomad/mods/*`
- Symbols/identifiers: `Transflow*` → `Mod*` (types, funcs, vars, constants). Keep type aliases for one release to reduce churn (e.g., `type TransflowRunner = ModRunner`).
- Event step names: keep behavior, only rename phase labels/messages where they include the feature name.

4. Storage keys and artifacts
- Write under `artifacts/mods/{id}/...` and `mods/{id}/...` (non-artifacts).
- Delete current Transflow artifacts under `artifacts/transflow/*` as part of the migration.
- Update SeaweedFS policy docs (docs/mods/knobs.md → docs/mods/knobs.md) to reflect new prefixes.

5. Tests and E2E
- Update unit/integration tests to new routes and storage keys.
- E2E: Switch controller base to `/v1/mods` endpoints; ensure `JavaMigrationComplete` remains green.

6. Docs & Comms
- Update `docs/api/transflow.md` → `docs/api/mods.md` with RESTful mapping table.
- Update `docs/FEATURES.md`, `README.md`, examples, and references in roadmap and internal docs.
- Add CHANGELOG entry for rename; no deprecation window required.

Endpoint Mapping (reference)

- POST `/v1/mods/run` ⇒ POST `/v1/mods`
- GET `/v1/mods/status/{id}` ⇒ GET `/v1/mods/{id}/status`
- GET `/v1/mods/list` ⇒ GET `/v1/mods`
- DELETE `/v1/mods/{id}` ⇒ DELETE `/v1/mods/{id}`
- GET `/v1/mods/artifacts/{id}` ⇒ GET `/v1/mods/{id}/artifacts`
- GET `/v1/mods/artifacts/{id}/{name}` ⇒ GET `/v1/mods/{id}/artifacts/{name}`
- POST `/v1/mods/event` ⇒ POST `/v1/mods/{id}/events`
- GET `/v1/mods/logs/{id}` ⇒ GET `/v1/mods/{id}/logs{?follow=true|false}`

CLI Mapping

- `ploy mod run` → `ploy mod run`
- `ploy mod watch` → `ploy mod watch`
- `ploy mod status` → `ploy mod status`

Rollout Strategy (single cutover)

- Rename code, endpoints, CLI, and artifacts in one release.
- Remove legacy endpoints and code.
- Purge Transflow artifacts in storage.

Validation Checklist

- Build + unit tests green locally (make fmt, make lint, go test ./...).
- API handler tests updated (route paths, SSE logs, artifacts endpoints).
- CLI tests updated for `mod` group.
- E2E JavaMigrationComplete green against Dev API (`/v1/mods`).
- Legacy alias tests pass while alias flag enabled.

Risk & Mitigations

- Endpoint drift: maintain mapping table in docs; update all call sites in repo.
- Artifact lookups: ensure all code paths use new `mods/` prefixes; purge old `transflow/` artifacts.

Work Items (Trackable)

- [ ] Implement `/v1/mods` routes + handlers and remove `/v1/mods/*`
- [ ] Add POST `/v1/mods/:id/events` and wire reporter
- [ ] CLI: add `mod` group; remove `transflow` group
- [ ] Mechanical rename (internal packages, symbols, files)
- [ ] Update docs: API, FEATURES, examples
- [ ] Update tests: unit/integration/E2E to new naming
- [ ] Purge `artifacts/transflow/*` and update SeaweedFS policy

Operator Runbook (Smoke/E2E)

1. Deploy Dev API on feature branch
2. Run E2E against `/v1/mods` controller base:
   ```bash
   E2E_LOG_CONFIG=1 PLOY_CONTROLLER=https://api.dev.ployman.app/v1 \
   E2E_REPO=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
   E2E_BRANCH=e2e/success \
   go test ./tests/e2e -tags e2e -v -run JavaMigrationComplete -timeout 20m
   ```
3. Verify: run id, steps, logs SSE, artifacts under `artifacts/mods/{id}/...`
