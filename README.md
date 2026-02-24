# Ploy — Migs Orchestrator and Control Plane

[![CI](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml) [![Coverage (native)](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml) [![Test Suite](https://github.com/iw2rmb/ploy/actions/workflows/test.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/test.yml) [![Build](https://github.com/iw2rmb/ploy/actions/workflows/build.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/build.yml)

Ploy is a workstation‑first orchestration stack for code‑mig (Migs) workflows. It consists of:

- `ploy` — a CLI for submitting migs runs, following logs, and administering clusters.
- `ployd` — the control-plane daemon with scheduler, HTTP/SSE API, PKI, and PostgreSQL-backed storage.
- `ployd-node` — lightweight worker nodes that execute jobs in ephemeral workspaces.

Ploy uses a server/node split with PostgreSQL for state and mTLS for all control‑plane traffic. Nodes clone repositories shallow on-demand and upload diffs/logs/artifacts back to PostgreSQL.

**High-Level Architecture**
- Control plane:
  - `ployd` exposes a `/v1/migs` facade for migs runs and a small set of cluster/control endpoints (PKI, nodes, repos).
  - Runs are stored in PostgreSQL along with jobs, logs, diffs, and artifact bundles.
- Workers:
  - `ployd-node` claims jobs from the control plane, hydrates an ephemeral workspace, runs migs/build/tests, and streams logs/diffs/artifacts to the server.
  - Workers derive MR branches and repo metadata (e.g., `ploy/{run_name|run_id}` when `--repo-target-ref` is omitted) from the run they execute.
- Security:
  - PKI-backed mTLS between CLI, server, and nodes; cluster CA and node/server certificates are managed by `ployd`.
  - Certificates carry roles (CLI, control‑plane, worker) via subject fields; authorization is enforced by the server.

For the detailed API surface and schemas, see `docs/api/OpenAPI.yaml`. This README focuses on what Ploy is, how to deploy it, and how to use it from the CLI.

**Docs You'll Want**
- Overview & quick start: this `README.md`.
- Local Docker cluster: `docs/how-to/deploy-locally.md`.
- Control‑plane APIs: `docs/api/OpenAPI.yaml` (authoritative schemas).
- Environment variables: `docs/envs/README.md`.
- Migs lifecycle and SSE events: `docs/migs-lifecycle.md`.
- Contributor rules and TDD discipline: `AGENTS.md`, `docs/testing-workflow.md`.

**Installation**

For end users, install the Ploy CLI using one of these methods:

**Homebrew (macOS/Linux)**
```bash
brew install iw2rmb/ploy/ploy
```

**Direct Download**

Download pre-built binaries from the [latest release](https://github.com/iw2rmb/ploy/releases/latest):
- `ploy` — CLI for workstations (Linux, macOS, Windows)
- `ployd` — Server daemon (Linux, macOS)
- `ployd-node` — Worker node daemon (Linux, macOS)

Extract and move the binary to a directory in your `PATH` (e.g., `/usr/local/bin`).

**Build**
- Requirements: Go 1.25+. Docker is optional for local step execution; node container execution is scaffolded in this slice.
- Build binaries from source into `dist/`:

  ```bash
  make build
  ```

This produces `dist/ploy` and `dist/ployd`.

Configuration: run `dist/ployd --config /path/to/ployd.yaml` or set `PLOYD_CONFIG_PATH` to change the default (`/etc/ploy/ployd.yaml`). The flag overrides the environment variable when both are provided.

**Listeners**
- Local Docker control plane: `http://localhost:8080` (bearer token auth). Health at `/health`.
- Metrics: `http://localhost:9100/metrics`.

**Scheduler & TTL**
- Background tasks run under `internal/server/scheduler`.
- TTL cleanup (`internal/store/ttlworker`) purges old rows and can drop monthly partitions.
- YAML (`scheduler` section):
  - `ttl`: retention for logs/events/diffs/artifact_bundles (default 30d if unset)
  - `ttl_interval`: how often the cleanup runs (default 1h if unset)
  - `drop_partitions`: when true, drop whole monthly partitions older than `ttl`

**Quick Start (Local Docker)**
- Deploy the local Docker stack (server + node + garage services) and write a local CLI descriptor:

  ```bash
  export PLOY_DB_DSN='postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable'
  ./deploy/local/run.sh
  export PLOY_CONFIG_HOME="$PWD/deploy/local/cli"
  ```

- Submit a mig run and follow events:

  ```bash
  ./dist/ploy mig run --repo-url https://github.com/example/repo.git \
    --repo-base-ref main \
    --follow
  ```

  # Optional: include an explicit target ref to control the MR source branch
  # (otherwise the node derives ploy/{run_name|run_id} when an MR is created).
  ./dist/ploy mig run --repo-url https://github.com/example/repo.git \
    --repo-base-ref main \
    --repo-target-ref feature/upgrade \
    --mr-success \
    --follow

- Stream run logs via SSE:

  ```bash
  ./dist/ploy run logs <run-id>
  ```

**Tests & Coverage**
- Run unit tests: `make test`
- Generate unit test coverage report (ensure ≥60% overall): `make test-coverage`
- Generate broader coverage report (all packages): `make coverage-all`
- Full local CI bundle (format, vet, staticcheck if installed, coverage gates): `make ci-check`

**Environment Variables**
- Full reference: `docs/envs/README.md`
- Key variables:
  - `PLOY_POSTGRES_DSN` — PostgreSQL DSN for the server.
  - `PLOY_CONFIG_HOME` — CLI config home; local Docker uses `./deploy/local/cli`.
  - `PLOY_BUILDGATE_IMAGE` — Optional Build Gate container image override.

**Contributing**
- Follow `AGENTS.md` and `docs/testing-workflow.md` (RED→GREEN→REFACTOR cadence; `make test` runs `go test ./internal/... ./cmd/...`).
- Keep docs in sync; update `README.md` and `docs/` as needed.

License: see `LICENSE` when present.
