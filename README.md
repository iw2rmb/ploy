# Ploy — Mods Orchestrator and Control Plane

Ploy is a workstation‑first orchestration stack for code‑mod (Mods) workflows. It consists of:

- `ploy` — a CLI for submitting Mods, following logs, managing runs, and administering clusters.
- `ployd` — the control-plane daemon with scheduler, API, and PostgreSQL-backed storage. (Previously called `ployd-server`; both names refer to the same binary.)
- `ployd-node` — lightweight worker nodes that execute jobs in ephemeral workspaces.

**Architecture**: Ploy uses a server/node split with PostgreSQL for state and mTLS-only authentication.
etcd and IPFS Cluster have been removed. Nodes clone repositories shallow on-demand and upload
diffs/logs/artifacts to the server's PostgreSQL database.

See `SIMPLE.md` for the detailed architecture, deployment topology, and migration notes.

Note on docs consolidation (2025‑11‑01): prior exploration files (ARCHITECTURE_DIAGRAM.md, CODEBASE_EXPLORATION.md, EXPLORATION_INDEX.md, EXPLORATION_README.md) were folded into this README and SIMPLE.md to reduce duplication and confusion.

**What Changed (2025‑11 — Postgres/mTLS Pivot)**
- **Server/Node Split**: Separate `ployd` (control-plane) and `ployd-node` (worker) binaries.
- **PostgreSQL**: Replaces etcd for state; stores runs, logs, diffs, and artifact bundles.
- **mTLS Only**: Bearer token auth removed; all communication uses mutual TLS.
- **No IPFS**: Artifacts stored in PostgreSQL; nodes clone repos shallow on-demand.
- **Simplified Deployment**: `ploy server deploy` and `ploy node add` CLI commands.

**Core Components**
- CLI entrypoint: `cmd/ploy` (commands: `server`, `node`, `mod`, `mods`, `runs`, `knowledge-base`).
- Server daemon: `cmd/ployd` with PostgreSQL (`pgx/v5` + `sqlc`), scheduler, and PKI.
- Node daemon: `cmd/ployd-node` with ephemeral workspaces, Build Gate, and mTLS client.
- Control‑plane HTTP/SSE: handlers in `internal/api/httpserver/*` and OpenAPI in `docs/api/OpenAPI.yaml`.
- Scheduler: In-DB queue using `FOR UPDATE SKIP LOCKED` on `runs.status='queued'`.
- Build Gate: `internal/workflow/buildgate/*` (sandbox runner, static checks, Java executor).
- Storage: PostgreSQL migrations in `internal/store/migrations/`, queries in `internal/store/queries/`.
- PKI: Cluster CA issues certificates; nodes submit CSRs via `/v1/pki/sign`.

**Docs You'll Want**
- Architecture: `SIMPLE.md` (server/node pivot, PostgreSQL, mTLS)
- Deployment: `docs/how-to/deploy-a-cluster.md`
- Roadmap: `ROADMAP.md` (migration checklist)
- Control‑plane APIs: `docs/api/OpenAPI.yaml`
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
- Requirements: Go 1.25+, Docker 28.x for local step execution.
- Build binaries into `dist/`:
  
  ```bash
  make build
  ```

This produces `dist/ploy` and `dist/ployd` (plus a Linux `ployd` for remote installs).

Configuration: run `dist/ployd --config /path/to/ployd.yaml` or set `PLOYD_CONFIG_PATH` to change the default (`/etc/ploy/ployd.yaml`). The flag overrides the environment variable when both are provided.

**Listeners**
- API: `:8443` (TLS; optionally mTLS when configured). Health at `/health`.
- Metrics: `:9100` (plain HTTP) exposing Prometheus at `/metrics`.

**Scheduler & TTL**
- Background tasks run under `internal/api/scheduler`.
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

- Add worker nodes to the cluster:

  ```bash
  dist/ploy node add --cluster-id <cluster-id> --address <host-or-ip>
  ```

- Submit a Mods run and follow events:

  ```bash
  dist/ploy mod run --repo-url https://github.com/example/repo.git \
    --repo-base-ref main --repo-target-ref feature-branch \
    --follow
  ```

 - Follow job logs via SSE:

  ```bash
  dist/ploy jobs follow <job-id>
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
- Keep docs in sync; update `SIMPLE.md`, `ROADMAP.md`, and `docs/` as needed.

**Legacy Removed (November 2025)**
- **etcd**: Replaced with PostgreSQL for all state.
- **IPFS Cluster**: Artifacts now stored in PostgreSQL; repos cloned shallow on-demand.
- **Token auth**: mTLS-only; bearer tokens removed.
- **Node labels**: Replaced with resource-snapshot scheduling.
- **SSH tunnels**: CLI uses direct HTTPS/mTLS to control-plane.

License: see `LICENSE` when present.
