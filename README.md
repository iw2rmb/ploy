# Ploy — Mods Orchestrator and Control Plane

Ploy is a workstation‑first orchestration stack for code‑mod (Mods) workflows. It consists of:

- `ploy` — a CLI for submitting Mods, following logs, managing runs, and administering clusters.
- `ployd` — the control-plane daemon with scheduler, API, and PostgreSQL-backed storage. (Previously called `ployd-server`; both names refer to the same binary.)
- `ployd-node` — lightweight worker nodes that execute jobs in ephemeral workspaces.

**Architecture**: Ploy uses a server/node split with PostgreSQL for state and mTLS-only authentication.
etcd and IPFS Cluster have been removed. Nodes clone repositories shallow on-demand and upload
diffs/logs/artifacts to the server's PostgreSQL database.

Note on architecture pivot (November 2025): the project moved to a server/node
split with PostgreSQL and mTLS, removed etcd/IPFS, and adopted a simpler
control‑plane API. This README is now the canonical architecture overview.
Prior exploration docs remain removed.

**What Changed (2025‑11 — Postgres/mTLS Pivot)**
- **Server/Node Split**: Separate `ployd` (control-plane) and `ployd-node` (worker) binaries.
- **PostgreSQL**: Replaces etcd for state; stores runs, logs, diffs, and artifact bundles.
- **mTLS Only**: Bearer token auth removed; all communication uses mutual TLS.
- **No IPFS**: Artifacts stored in PostgreSQL; nodes clone repos shallow on-demand.
- **Simplified Deployment**: `ploy server deploy` and `ploy node add` CLI commands.

**Core Components**
- CLI entrypoint: `cmd/ploy` (notable commands: `server`, `node`, `rollout`, `mod`, `mods`, `runs`, `upload`).
- Server daemon: `cmd/ployd` with PostgreSQL (`pgx/v5` + `sqlc`), scheduler, and PKI.
- Node daemon: `cmd/ployd-node` with ephemeral workspaces and mTLS client; streams logs/diffs/artifacts.
- Control‑plane HTTP/SSE: handlers in `internal/server/handlers/*`, HTTP server in `internal/server/http/*`, SSE hub in `internal/stream`.
- Scheduler: In‑DB queue; nodes claim runs with advisory‑lock semantics (see `internal/store/*`).
- Build Gate + execution: execution scaffolding lives under `internal/workflow/runtime/step`; Java build‑gate health check in `internal/worker/lifecycle/health_buildgate.go`.
- Storage: migrations in `internal/store/migrations/`, queries in `internal/store/queries/`.
- PKI: Cluster CA issues certificates; nodes and admins submit CSRs via `/v1/pki/sign*` endpoints.

**Authentication & Roles (mTLS)**
- Certificates carry role in Subject OU (`Ploy role=<role>`) or via CN prefix. Implemented roles:
  - `cli-admin` — administrative CLI; allowed on admin endpoints and standard control‑plane.
  - `client` (alias: control‑plane) — non‑admin CLI; allowed on standard control‑plane.
  - `worker` (alias: node) — node agent; allowed on worker ingest endpoints.
- Extracted from cert OU or CN. CNs like `node:<uuid>` are treated as `worker`. Admin is a superset of control‑plane for authorization.

**API Overview (current implementation)**
- PKI: `POST /v1/pki/sign`, `POST /v1/pki/sign/admin`, `POST /v1/pki/sign/client`.
- Repos: `POST /v1/repos`, `GET /v1/repos`, `GET /v1/repos/{id}`, `DELETE /v1/repos/{id}`.
- Mods (catalog): `POST /v1/mods/crud`, `GET /v1/mods/crud[?repo_id=]`, `GET /v1/mods/crud/{id}`, `DELETE /v1/mods/crud/{id}`.
- Runs: `POST /v1/runs`, `GET /v1/runs` (collection/timings), `GET /v1/runs/{id}` (status), `GET /v1/runs/{id}/events` (SSE), `GET /v1/runs/{id}/timing`.
- Ingest (worker): `POST /v1/runs/{id}/diffs`, `POST /v1/runs/{id}/logs`, `POST /v1/runs/{id}/artifact_bundles`.
- Nodes (control): `GET /v1/nodes`, `POST /v1/nodes/{id}/drain`, `POST /v1/nodes/{id}/undrain`.
- Nodes (worker): `POST /v1/nodes/{id}/heartbeat`, `POST /v1/nodes/{id}/claim`, `POST /v1/nodes/{id}/ack`, `POST /v1/nodes/{id}/complete`, `POST /v1/nodes/{id}/events`, `POST /v1/nodes/{id}/logs`, `POST /v1/nodes/{id}/stage/{stage}/diff`, `POST /v1/nodes/{id}/stage/{stage}/artifact`.

Note: An API simplification to a `/v1/mods`‑only facade (submit/status/events/artifacts) is planned; see `ROADMAP.md`. Until that lands, the server exposes the routes above.

**Architecture packages (boundaries)**
- `internal/stream`: shared SSE hub and HTTP helpers (server + node agent use it).
- `internal/worker`: node‑side execution primitives (`jobs`, `lifecycle`, `hydration`).
- `internal/nodeagent`: node daemon composition and HTTP server/handlers.

**Docs You'll Want**
- Architecture: this `README.md` (pivot summary and current API)
- Deployment: `docs/how-to/deploy-a-cluster.md`
- Updating a cluster: `docs/how-to/update-a-cluster.md` (rolling updates via `ploy rollout`)
- Roadmap: `ROADMAP.md` (current plan and latest slices)
- Control‑plane APIs: `docs/api/OpenAPI.yaml` (will be updated alongside the `/v1/mods` simplification)
- Environment variables: `docs/envs/README.md`
- Engineering rules: `GOLANG.md`

**Docs Map**
- Overview & quick start: this `README.md` (canonical entry).
- Deep architecture: `SIMPLE.md` (source of truth for diagrams/flows).
- APIs: `docs/api/OpenAPI.yaml` (authoritative endpoints and schemas).
- How‑tos: `docs/how-to/*.md` (deploy/update guides).
- Envs: `docs/envs/README.md` (canonical env reference).
- Contributor process: `AGENTS.md` (TDD, coverage, docs policy).

**Build**
- Requirements: Go 1.25+. Docker is optional for local step execution; node container execution is scaffolded in this slice.
- Build binaries into `dist/`:
  
  ```bash
  make build
  ```

This produces `dist/ploy` and `dist/ployd` (plus a Linux `ployd` for remote installs).

Configuration: run `dist/ployd --config /path/to/ployd.yaml` or set `PLOYD_CONFIG_PATH` to change the default (`/etc/ploy/ployd.yaml`). The flag overrides the environment variable when both are provided.

**Listeners**
- API: `:8443` (TLS 1.3 with mandatory mTLS). Health at `/health`.
- Metrics: `:9100` (plain HTTP) exposing Prometheus at `/metrics`.

**Scheduler & TTL**
- Background tasks run under `internal/server/scheduler`.
- TTL cleanup (`internal/store/ttlworker`) purges old rows and can drop monthly partitions.
- YAML (`scheduler` section):
  - `ttl`: retention for logs/events/diffs/artifact_bundles (default 30d if unset)
  - `ttl_interval`: how often the cleanup runs (default 1h if unset)
  - `drop_partitions`: when true, drop whole monthly partitions older than `ttl`

**Quick Start**
- Deploy the control-plane server (installs PostgreSQL if `--postgresql-dsn` not provided):

  ```bash
  dist/ploy server deploy --address <host-or-ip>
  ```

  Notes:
  - By default, the command attempts to detect and reuse an existing cluster CA/server certificate on the host (`--reuse=true`).
  - To force a fresh PKI, pass `--force-new-ca` (or `--reuse=false`).
  - `--refresh-admin-cert` is reserved for a follow-up slice and is currently ignored with a warning.

- Add worker nodes to the cluster:

  ```bash
  dist/ploy node add --cluster-id <cluster-id> --address <host-or-ip> --server-url https://<server-host>:8443
  ```

- Submit a Mods run and follow events:

  ```bash
  dist/ploy mod run --repo-url https://github.com/example/repo.git \
    --repo-base-ref main --repo-target-ref feature-branch \
    --follow
  ```

 - Follow ticket events via SSE:

  ```bash
  dist/ploy mods logs <ticket-id>
  ```

**Tests & Coverage**
- Run unit tests with coverage: `make test`
- Enforce ≥60% overall coverage: `make test-coverage-threshold`
- Enforce ≥90% on critical paths (scheduler/PKI/ingest): `make test-coverage-critical`
- Full local CI bundle (format, vet, staticcheck if installed, coverage gates): `make ci-check`

**Environment Variables**
- Full reference: `docs/envs/README.md`
- Key variables:
  - `PLOY_SERVER_PG_DSN` — PostgreSQL DSN for the server (e.g., `postgres://user:pass@localhost:5432/ploy`).
  - `PLOY_CONTROL_PLANE_URL` — Override control-plane URL (descriptors preferred).
  - `PLOY_SERVER_CA_CERT` / `PLOY_SERVER_CA_KEY` — Cluster CA for PKI operations.
  - `PLOY_BUILDGATE_JAVA_IMAGE` — Optional Java image for the Build Gate.

**Contributing**
- Follow `GOLANG.md` and `AGENTS.md` (RED→GREEN→REFACTOR cadence; `make test` runs `go test -cover ./...`).
- Keep docs in sync; update `README.md`, `ROADMAP.md`, and `docs/` as needed.

**Legacy Removed (November 2025)**
- **etcd**: Replaced with PostgreSQL for all state.
- **IPFS Cluster**: Artifacts now stored in PostgreSQL; repos cloned shallow on-demand.
- **Token auth**: mTLS-only; bearer tokens removed.
- **Node labels**: Replaced with resource-snapshot scheduling.
- **SSH tunnels**: CLI uses direct HTTPS/mTLS to control-plane.

License: see `LICENSE` when present.
