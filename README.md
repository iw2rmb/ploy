# Ploy — Migs Orchestrator and Control Plane

[![CI](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/ci.yml) [![Coverage (native)](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/coverage.yml) [![Test Suite](https://github.com/iw2rmb/ploy/actions/workflows/test.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/test.yml) [![Build](https://github.com/iw2rmb/ploy/actions/workflows/build.yml/badge.svg)](https://github.com/iw2rmb/ploy/actions/workflows/build.yml)

Ploy is a workstation‑first orchestration stack for code‑mig (Migs) workflows. It consists of:

- `ploy` — a CLI for submitting migs runs, following logs, and administering clusters.
- `ployd` — the control-plane daemon with scheduler, HTTP/SSE API, PKI, and PostgreSQL-backed storage.
- `ployd-node` — lightweight worker nodes that execute jobs in ephemeral workspaces.

Ploy uses a server/node split with PostgreSQL for state and mTLS for all control‑plane traffic. Nodes clone repositories shallow on-demand and upload diffs/logs/artifacts back to PostgreSQL.

## Install

Homebrew (macOS/Linux):
```bash
brew install --cask iw2rmb/ploy/ploy
```

Direct download: [latest release](https://github.com/iw2rmb/ploy/releases/latest)

Build from source:
```bash
make build
```

## Quick Start

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable'
export PLOY_OBJECTSTORE_ENDPOINT='http://localhost:3900'
export PLOY_OBJECTSTORE_ACCESS_KEY='...'
export PLOY_OBJECTSTORE_SECRET_KEY='...'
export PLOY_VERSION='v0.1.0'                  # optional; defaults to ./VERSION
export PLOY_CA_CERTS='/path/to/ca-bundle.pem' # optional

ploy cluster deploy
```

Submit and follow a run:

```bash
ploy mig run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --follow
```

## Documentation

- Deploy cluster: `docs/how-to/deploy.md`
- API schema: `docs/api/OpenAPI.yaml`
- Environment variables: `docs/envs/README.md`
- Migs lifecycle: `docs/migs-lifecycle.md`
- TUI usage: `docs/how-to/tui-navigation.md`

License: see `LICENSE` when present.
