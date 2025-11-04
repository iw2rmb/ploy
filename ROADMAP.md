# API Simplification — Replace /v1/runs with /v1/mods, remove /v1/mods/crud

Scope: Consolidate the external control‑plane API under the "mods" facade. Eliminate the mods catalog CRUD surface and the separate "/v1/runs" read/stream paths. Users submit a Mods run via POST /v1/mods and then interact at /v1/mods/{id}[/*] for status, events, and uploads. No backward compatibility is required; the only deployment is the VPS lab.

Documentation:
- Design context: SIMPLE.md (server/node pivot), README.md (API overview)
- Current API: docs/api/OpenAPI.yaml (to be updated in this slice)
- How‑tos: docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md
- Template used: ../auto/ROADMAP.md

Legend: [ ] todo, [x] done.

## Phase 1 — Introduce ticket endpoints; remove mods catalog
- [x] Add ticket submission endpoint — POST /v1/mods — Accepts TicketSubmitRequest, returns TicketSummary (ticket_id == run UUID)
  - Change: implement handler (new file, e.g., `internal/server/handlers/handlers_mods_ticket.go`), register in `internal/server/handlers/register.go`; map request → create run row; accept repo URL/refs directly (no pre‑registered mod/repo required).
  - Test: unit tests for 200/202/4xx paths; verify JSON schema enforced; OpenAPI test updated to include /v1/mods.

- [x] Add ticket status endpoint — GET /v1/mods/{id} — Returns TicketSummary by run UUID
  - Change: implement handler; fetch run and shape TicketSummary (stages map may be stubbed initially from existing run/stage rows).
  - Test: unit tests for happy/missing/invalid UUID; ensure timestamps and optional fields serialized consistently.

- [x] Add events streaming — GET /v1/mods/{id}/events — Native SSE under mods (no proxy)
  - Change: register new route wired to existing SSE hub; remove reliance on /v1/runs/{id}/events in the CLI.
  - Test: SSE unit/integration test (resume via Last-Event-ID, basic stream lifecycle) updated to the new path.

- [x] Move artifact bundle ingest — POST /v1/mods/{id}/artifact_bundles — replace /v1/runs/{id}/artifact_bundles
  - Change: register new path and point to existing handler; mark old route for removal in Phase 2.
  - Test: adjust handler tests to use new path; verify size cap (≤1 MiB gzipped) maintained.

- [x] Remove the mods catalog surface — delete /v1/mods/crud and related code
  - Change: drop handlers in `internal/server/handlers/handlers_repos_mods.go` for mods CRUD; keep repos CRUD (if still useful) or gate for future removal.
  - Test: delete/adjust unit tests; sweep OpenAPI to remove /v1/mods/crud and related schemas/paths.

- [x] OpenAPI and docs update
  - Change: docs/api/OpenAPI.yaml — remove /v1/mods/crud; add /v1/mods (POST, GET), /v1/mods/{id}/events, /v1/mods/{id}/artifact_bundles. Update security/roles notes.
  - Test: `docs/api/verify_openapi_test.go` updated list of endpoints; schema sanity tests remain green.

## Phase 2 — Retire /v1/runs facade (server + CLI)
- [x] Remove read/stream routes under /v1/runs
  - Change: delete /v1/runs (POST, GET), /v1/runs/{id}, /v1/runs/{id}/events registrations; keep internal store tables (runs/stages) intact.
  - Test: update handlers tests to only use mods paths; verify SSE and status routes pass.

- [x] CLI alignment — point Mods commands to /v1/mods
  - Change: `cmd/ploy/mod_run.go` and `internal/cli/mods/*`: change submission to POST /v1/mods; change follow to GET /v1/mods/{id}/events; adjust status/inspect/cancel/resume to /v1/mods/{id}[/*].
  - Test: CLI unit tests updated; manual smoke: `dist/ploy mod run --repo-url <repo> --repo-base-ref main --repo-target-ref mods-upgrade-java17 --follow` succeeds against the lab.

- [x] Artifact download surface (optional but recommended)
  - Change: implement GET /v1/artifacts?cid=… and GET /v1/artifacts/{id} for CLI downloads (currently referenced by CLI); or switch CLI to fetch via a simpler ticket artifacts listing under /v1/mods/{id}/artifacts.
  - Test: CLI `--artifact-dir` path succeeds; server returns 200 with correct payload and limits.

## Phase 3 — Docs sweep and E2E
- [x] Docs sweep: README.md, docs/how-to/deploy-a-cluster.md, tests/mod/README.md
  - Change: replace references to /v1/runs with /v1/mods; remove /v1/mods/crud examples; clarify the simple flow (submit → events → status → artifacts).
  - Test: `rg` sweep shows no stale routes; how‑to snippets run cleanly.

## Phase 4 — Cleanup and deprecations
- [x] Remove dead code and tables (optional)
  - Change: if desired, inline repo/spec fields into the run submission and drop the mods table + FK; or keep tables as internal implementation details.
  - Test: migrations updated; unit/integration tests green; coverage ≥60% overall and ≥90% on critical packages per AGENTS.md.

## Acceptance Criteria
- CLI “mod run … --follow” works end‑to‑end against VPS lab with only /v1/mods/* routes.
- No /v1/mods/crud or /v1/runs reads/streams exposed in OpenAPI or server router.
- Unit tests updated and passing; OpenAPI verification tests green; coverage thresholds preserved.
- Docs (README/how‑to/OpenAPI) reflect the simplified API.
