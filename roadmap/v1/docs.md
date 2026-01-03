# Roadmap v1 — Docs

## Deltas vs HEAD

- Change: separate “implemented docs” from “planned docs”.
  - Where: move v1 planned docs from `docs/` to `roadmap/v1/` and keep `docs/` normative for current behavior.
  - Compatibility impact: none (documentation-only), but links and references must be updated.

## 0) Primary goal

Make docs consistent with the v1 CLI/API direction:

- `ploy mod ...` manages **mod projects** (name, spec variants, repo set).
- `ploy run ...` submits **single-repo runs** (immediate execution) and creates a mod project as a side-effect (`mod.name == mod.id`).
- `ploy mod run <mod> ...` runs a **mod project** (immediate execution over the mod repo set).

## 1) Canonical docs tree (proposal)

Keep docs split by: **architecture**, **CLI reference**, **how-to workflows**, **API**.

```
docs/
  README.md

  architecture/
    data-model.md
    mods-lifecycle.md

  cli/
    README.md
    mod.md
    run.md

  how-to/
    deploy-a-cluster.md
    update-a-cluster.md
    create-mr.md
    publish-mods.md
    cancel-endpoint-rollout.md
    descriptor-https-quickstart.md

  envs/
    README.md
```

Notes:
- Keep OpenAPI under `docs/api/` as-is, but align its paths + semantics.
- Move long CLI usage prose out of architecture docs.

## 2) Required edits for v1 alignment

### 2.1 Resolve `/v1/mods` collision before writing docs

v1 repurposes `mod` as a **project**:

- Current reality: `POST /v1/mods` submits a Mods run (`docs/api/paths/mods.yaml`, `internal/server/handlers/mods_ticket.go`).
- v1 decision: `POST /v1/mods` creates a mod project (`roadmap/v1/api.md`).

This also collides with existing “run artifacts” endpoints that currently live under `/v1/mods/{run_id}/*`:

- `GET /v1/mods/{run_id}/diffs` (and other `/v1/mods/{run_id}/*` run-scoped routes) must move under `/v1/runs/{run_id}/*`.
- For multi-repo runs, repo-scoped artifacts must be addressed under `/v1/runs/{run_id}/repos/{repo_id}/...`.

Docs and OpenAPI must be rewritten to match the chosen outcome:

**Chosen direction (v1):**
- Move single-repo run submission from `POST /v1/mods` → `POST /v1/runs` (used by `ploy run --spec --repo ...`).
- Use `/v1/mods/*` for mod project/spec/repo management.
- Add `POST /v1/mods/{mod_id}/runs` for executing a mod project (used by `ploy mod run <mod> ...`).
- Use `PATCH /v1/mods/{mod_id}/archive` and `/unarchive` for mod lifecycle state; archived mods cannot be executed.

### 2.2 Update “batch run” docs to the new model

Files with old semantics (examples to rewrite):

- `docs/mods-lifecycle.md`: batch workflow section currently uses `ploy mod run --name ...` and `mod run repo add ...`.
  - Replace with v1: `ploy mod add`, `ploy mod spec add`, `ploy mod repo import|add`, `ploy mod run <mod>`.
- `docs/how-to/deploy-a-cluster.md`: submission examples should match v1.
- `docs/how-to/create-mr.md`: batch MR workflow should start from `ploy mod ...` project setup.
- `cmd/ploy/README.md`: keep as developer-facing CLI reference, but align examples and remove stale subcommands.

### 2.3 OpenAPI coverage gaps

- Add missing endpoints that the CLI already uses:
  - `POST /v1/runs/{id}/start` is implemented and used by CLI (`internal/server/handlers/runs_batch_http.go`, `internal/cli/runs/start.go`) but is not present in `docs/api/OpenAPI.yaml`.
- Add new endpoints required by v1:
  - Add `POST /v1/runs` (submit single-repo run) and document request/response.
  - Add `/v1/mods` CRUD for mod projects (create/list/delete).
  - Add `POST /v1/mods/{mod_id}/runs` (execute mod project).
  - Add mod project `specs` and `repos` endpoints as defined in `roadmap/v1/api.md`.

## 3) Redundant / low-value content (recommend prune or relocate)

- `docs/envs/README.md` includes CLI flag documentation (e.g., `--spec`, `--name`).
  - Move to CLI reference docs (`docs/cli/*.md` or `cmd/ploy/README.md`).
- `docs/mods-lifecycle.md` contains long CLI how-to sequences.
  - Keep lifecycle + data model + invariants; move “how to use CLI” to `docs/cli/`.
- Any doc that duplicates Cobra `--help` output verbatim.
  - Prefer “concept + link to `ploy <cmd> --help`”.

## 4) TODO decisions to unblock implementation

- `ploy run --spec ... --repo ...` creates a mod project; the created mod has `name == id`.
- Run submission lives at `POST /v1/runs`; mod projects live at `POST /v1/mods`; executing a mod project is `POST /v1/mods/{mod_id}/runs`.
- Define v1 run artifacts APIs for multi-repo runs:
  - how `ploy mod pull` (repo-scoped) selects diffs/logs/events for a single repo in a run (`run_id + repo_id`).
