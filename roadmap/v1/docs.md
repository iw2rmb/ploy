# Roadmap v1 — Docs

## 0) Primary goal

Make docs consistent with the v1 CLI/API direction:

- `ploy mod ...` manages **mod projects** (name, spec variants, repo set).
- `ploy run ...` is the **execution surface** (submit single-repo runs, inspect runs, stream logs, etc.).
- Control-plane auth is **bearer token** in `Authorization: Bearer ...` (CLI + nodes).

## 1) Canonical docs tree (proposal)

Keep docs split by: **architecture**, **CLI reference**, **how-to workflows**, **API**.

```
docs/
  README.md

  architecture/
    auth.md
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
- Keep OpenAPI under `docs/api/` as-is, but align its auth + paths.
- Move long CLI usage prose out of architecture docs.

## 2) Auth: what the code actually does (must match docs)

### CLI → control plane

- CLI loads the default cluster descriptor (`internal/cli/config`) and injects:
  - `Authorization: Bearer <token>` when descriptor `token` is set (`cmd/ploy/common_http.go`).
- CLI does **not** present a client certificate (no mTLS from CLI).
- CLI TLS: if the descriptor address is `https://...`, it uses standard TLS without custom RootCAs (`cmd/ploy/common_http.go`).

### Node → control plane

- Nodes send `Authorization: Bearer <token>` and `PLOY_NODE_UUID: <node_id>` on requests (`internal/nodeagent/diffuploader.go`).
- Nodes *may* also use TLS client certs if node config enables TLS (`internal/nodeagent/config.go`):
  - Outgoing client TLS config loads `{cert,key,ca}` and sets RootCAs (`internal/nodeagent/diffuploader.go`).

### Node bootstrap

- Bootstrap uses a short-lived **bootstrap bearer token** (JWT) to call `POST /v1/pki/bootstrap` and exchange CSR for a cert (`internal/server/handlers/bootstrap.go`, `internal/nodeagent/agent.go`).

### Control plane server transport

- The API server currently listens on plain TCP (no TLS in-process); TLS termination is expected outside the process (`internal/server/http/server.go`).
- Server-side auth middleware accepts bearer tokens and has an mTLS fallback, but in the default server transport the mTLS path is not exercised (`internal/server/auth/authorizer.go`).

## 3) Required edits for v1 alignment

### 3.1 Fix auth inconsistencies (blocking)

- `docs/api/OpenAPI.yaml`:
  - Replace `mutualTLS` security scheme with HTTP bearer auth (or document both, but make bearer primary).
  - Update the introductory description: it currently says “mutual TLS … bearer removed”.
- `docs/envs/README.md`:
  - Remove/replace the pivot section that claims mTLS client auth was replaced, while other docs/API say otherwise.
  - Keep only *environment variables*; move CLI flags out.
- `docs/how-to/create-mr.md`:
  - Remove references to `(removed) PLOY_CONTROL_PLANE_URL` and replace with descriptor-derived control plane URL guidance.

### 3.2 Resolve `/v1/mods` collision before writing docs

If v1 repurposes `mod` as a project:

- Current reality: `POST /v1/mods` submits a Mods run (`docs/api/paths/mods.yaml`, `internal/server/handlers/mods_ticket.go`).
- v1 proposal: `POST /v1/mods` becomes “create mod project” (roadmap/v1/api.md).

Docs must reflect the chosen outcome. Two viable directions:

**Option A (recommended for v1):**
- Move run submission from `POST /v1/mods` → `POST /v1/runs`.
- Keep `/v1/mods/*` for mod project/spec/repo management.
- Update CLI docs to use `ploy run --repo-url ... --spec ...` for immediate single-repo runs.

**Option B:**
- Keep run submission at `/v1/mods` and put mod-project APIs under a new prefix (e.g., `/v1/mod-projects`).
- Keep CLI as `ploy mod run ...` for submission and introduce different term for project management.

### 3.3 Update “batch run” docs to the new model

Files with old semantics (examples to rewrite):

- `docs/mods-lifecycle.md`: batch workflow section currently uses `ploy mod run --name ...` and `mod run repo add ...`.
  - Replace with v1: `ploy mod add`, `ploy mod spec add`, `ploy mod repo import`, `ploy mod run <mod>`.
- `docs/how-to/deploy-a-cluster.md`: submission examples should match v1.
- `docs/how-to/create-mr.md`: batch MR workflow should start from `ploy mod ...` project setup.
- `cmd/ploy/README.md`: keep as developer-facing CLI reference, but align examples and remove stale subcommands.

### 3.4 OpenAPI coverage gaps

- Add missing endpoints that the CLI already uses:
  - `POST /v1/runs/{id}/start` is implemented and used by CLI (`internal/server/handlers/runs_batch_http.go`, `internal/cli/runs/start.go`) but is not present in `docs/api/OpenAPI.yaml`.
- If Option A is chosen:
  - Add `POST /v1/runs` (submit single-repo run) and document response contract.
  - Mark `POST /v1/mods` (run submission) as removed/renamed.

## 4) Redundant / low-value content (recommend prune or relocate)

- `docs/envs/README.md` includes CLI flag documentation (e.g., `--spec`, `--name`).
  - Move to CLI reference docs (`docs/cli/*.md` or `cmd/ploy/README.md`).
- `docs/mods-lifecycle.md` contains long CLI how-to sequences.
  - Keep lifecycle + data model + invariants; move “how to use CLI” to `docs/cli/`.
- Any doc that duplicates Cobra `--help` output verbatim.
  - Prefer “concept + link to `ploy <cmd> --help`”.

## 5) TODO decisions to unblock implementation

- Does `ploy run --repo-url ... --spec ...` create a mod project/spec variant automatically?
  - If yes: define naming/ID policy (avoid unbounded clutter).
  - If no: make it explicitly “ad-hoc run” with no mod linkage.
- Decide where run submission lives (`POST /v1/runs` vs `POST /v1/mods`), then rewrite docs accordingly.

