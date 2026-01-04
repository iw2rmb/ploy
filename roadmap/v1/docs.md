# Roadmap v1 — Docs

## 0) Primary goal

Keep documentation aligned with the v1 scope and execution model described in:

- `roadmap/v1/scope.md`
- `roadmap/v1/api.md`
- `roadmap/v1/cli.md`
- `roadmap/v1/db.md`
- `roadmap/v1/statuses.md`

Change entry: keep `docs/` normative for HEAD, keep v1 plans under `roadmap/v1/`.

- Current (HEAD): implemented behavior is documented in `docs/` (and OpenAPI in `docs/api/OpenAPI.yaml`).
- Proposed (v1): all planned/unimplemented behavior for v1 stays under `roadmap/v1/` until implemented, then moves into `docs/` and `roadmap/v1/` is deleted.
- Where: documentation-only; policy is defined in `AGENTS.md` (“Documentation Layout Policy”).
- Compatibility: none (docs layout only).
- Unchanged: `docs/` remains the source of truth for current behavior at HEAD.

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

This is defined in `roadmap/v1/scope.md` (“`/v1/mods/*` route collisions”) and `roadmap/v1/api.md` (canonical v1 API surface).

### 2.2 Update “batch run” docs to the new model

Files with old semantics (examples to rewrite):

- `docs/mods-lifecycle.md`: batch workflow section currently uses `ploy mod run --name ...` and `mod run repo add ...`.
  - Replace with v1: `ploy mod add`, `ploy mod spec set`, `ploy mod repo import|add`, `ploy mod run <mod>`.
- `docs/how-to/deploy-a-cluster.md`: submission examples should match v1.
- `docs/how-to/create-mr.md`: batch MR workflow should start from `ploy mod ...` project setup.
- `cmd/ploy/README.md`: keep as developer-facing CLI reference, but align examples and remove stale subcommands.

### 2.3 OpenAPI coverage gaps

- `docs/api/OpenAPI.yaml` must be updated to match the canonical surface in `roadmap/v1/api.md`.

## 3) Redundant / low-value content (recommend prune or relocate)

- `docs/envs/README.md` includes CLI flag documentation (e.g., `--spec`, `--name`).
  - Move to CLI reference docs (`docs/cli/*.md` or `cmd/ploy/README.md`).
- `docs/mods-lifecycle.md` contains long CLI how-to sequences.
  - Keep lifecycle + data model + invariants; move “how to use CLI” to `docs/cli/`.
- Any doc that duplicates Cobra `--help` output verbatim.
  - Prefer “concept + link to `ploy <cmd> --help`”.

## 4) Decisions (v1)

Canonical decisions live in:

- `roadmap/v1/scope.md` (scope + entrypoints)
- `roadmap/v1/api.md` (API surface)
- `roadmap/v1/statuses.md` (status model + progression)
- `roadmap/v1/db.md` (DB shape)
