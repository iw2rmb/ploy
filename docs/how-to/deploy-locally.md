# Deploy Ploy Locally (Docker)

This guide brings up the local Ploy stack using host PostgreSQL plus containers for:
- Garage object store (`garage`) and bootstrap (`garage-init`)
- Control plane `ployd` (`server`)
- Worker `ployd-node` (`node`)
- Gradle cache (`gradle-build-cache`)

The local stack no longer runs a PostgreSQL container. The server uses your host PostgreSQL via `PLOY_DB_DSN`.

## Prerequisites

- Go 1.25+ (`make build` produces local binaries).
- Local PostgreSQL running and reachable from containers.
- `psql` and `pg_isready` available on the host.
- Docker Desktop / Docker Engine with Compose v2.

## Quickstart (Docker)

From repo root:

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable'
export PLOY_SERVER_PORT=18080   # optional; default 8080
./deploy/local/run.sh
export PLOY_CONFIG_HOME="$PWD/deploy/local/cli"
```

What the script does:
- Ensures PostgreSQL is reachable.
- Creates database `ploy` if missing.
- Optional `--drop-db` drops and recreates `ploy`.
- Builds local binaries (`make build`).
- Builds runtime images (including `garage-init`; no Go toolchain in image builds).
- Starts compose services.
- Generates admin/worker JWTs and inserts them into `api_tokens`.
- Seeds local node record.
- Writes local CLI descriptor in `deploy/local/cli/clusters/local.json`.
- Local Garage S3 credentials are preseeded in compose:
  - access key: `GK000000000000000000000001`
  - secret key: `0000000000000000000000000000000000000000000000000000000000000001`
  - region: `garage` (wired via `PLOY_OBJECTSTORE_REGION`)

## Script Flags

`./deploy/local/run.sh` supports:

```bash
--drop-db   # drop + recreate "ploy" before deploy
--ployd     # refresh/deploy server only
--nodes     # refresh/deploy node (+ required server dependency)
```

No flags means full deploy (server + node + garage services).

## Binary Mount Model

- Host binaries are mounted into containers:
  - `dist/ployd-linux -> /usr/local/bin/ployd`
  - `dist/ployd-node-linux -> /usr/local/bin/ployd-node`
- Runtime images are built from:
  - `deploy/images/server/Dockerfile.local`
  - `deploy/images/node/Dockerfile.local`
  - `deploy/local/garage/Dockerfile.init` (bootstrap helper image with `/garage` + shell)
- No Go compilation happens in container builds.

## Verify

- Server health:

```bash
curl -fsS "http://localhost:${PLOY_SERVER_PORT:-8080}/health"
```

- Metrics:

```bash
curl -fsS http://localhost:9100/metrics | head
```

- Token list (uses local descriptor):

```bash
PLOY_CONFIG_HOME="$PWD/deploy/local/cli" ./dist/ploy cluster token list
```

## Stop / Clean

```bash
docker compose -f deploy/local/docker-compose.yml down -v
```

For a full reset including DB recreation:

```bash
./deploy/local/run.sh --drop-db
```

## Troubleshooting

- `PLOY_DB_DSN` missing:
  - Set it before running scripts.
- Port `8080` already in use:
  - Set `PLOY_SERVER_PORT` (example: `18080`) before running scripts.
- DB unreachable:
  - Verify host PostgreSQL is running.
  - Do not use `localhost` in container DSN. Set `PLOY_DB_DSN` to a container-reachable host (for example `host.containers.internal`).
  - Verify DSN host is reachable from containers (`host.containers.internal` on macOS).
- Missing binaries:
  - Ensure `dist/ployd-linux` and `dist/ployd-node-linux` exist after `make build`.
- Node cannot run containers:
  - Ensure socket mount path is valid:
    - Docker script default: `/var/run/docker.sock`
  - Override explicitly if needed: `export PLOY_CONTAINER_SOCKET_PATH=/var/run/docker.sock`
- Logs:
  - `docker compose -f deploy/local/docker-compose.yml logs -f server`
  - `docker compose -f deploy/local/docker-compose.yml logs -f node`
  - `docker compose -f deploy/local/docker-compose.yml logs -f garage`

## Related

- `README.md`
- `docs/envs/README.md`
- `docs/how-to/token-management.md`
