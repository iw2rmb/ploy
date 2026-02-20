# Deploy Ploy Locally (Docker and Podman)

This guide brings up the local Ploy stack using host PostgreSQL plus containers for:
- Garage object store (`garage`) and bootstrap (`garage-init`)
- Control plane `ployd` (`server`)
- Worker `ployd-node` (`node`)
- Gradle cache (`gradle-build-cache`)

The local stack no longer runs a PostgreSQL container. The server uses your host PostgreSQL via `PLOY_LOCAL_PG_DSN`.

## Prerequisites

- Go 1.25+ (`make build` produces local binaries).
- Local PostgreSQL running and reachable from containers.
- `psql` and `pg_isready` available on the host.
- Docker path:
  - Docker Desktop / Docker Engine with Compose v2.
- Podman path:
  - `podman` and `podman-compose`.

## Quickstart (Docker)

From repo root:

```bash
export PLOY_LOCAL_PG_DSN='postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable'
./scripts/local-docker.sh
export PLOY_CONFIG_HOME="$PWD/local/cli"
```

What the script does:
- Ensures PostgreSQL is reachable.
- Creates database `ploy` if missing.
- Optional `--drop-db` drops and recreates `ploy`.
- Builds local binaries (`make build`).
- Builds runtime images (no Go toolchain in image builds).
- Starts compose services.
- Generates admin/worker JWTs and inserts them into `api_tokens`.
- Seeds local node record.
- Writes local CLI descriptor in `local/cli/clusters/local.json`.

## Quickstart (Podman)

From repo root:

```bash
export PLOY_LOCAL_PG_DSN='postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable'
./scripts/local-podman.sh
export PLOY_CONFIG_HOME="$PWD/local/cli"
```

Defaults:
- Compose command: `podman-compose -f local/docker-compose.yml`
- Container engine command: `podman`
- You can override with `COMPOSE_CMD` and `CONTAINER_ENGINE`.

## Script Flags

Both scripts support:

```bash
--drop-db   # drop + recreate "ploy" before deploy
--ployd     # refresh/deploy server only
--nodes     # refresh/deploy node only
```

No flags means full deploy (server + node + garage services).

## Binary Mount Model

- Host binaries are mounted into containers:
  - `dist/ployd-linux -> /usr/local/bin/ployd`
  - `dist/ployd-node-linux -> /usr/local/bin/ployd-node`
- Runtime images are built from:
  - `docker/server/Dockerfile.local`
  - `docker/node/Dockerfile.local`
- No Go compilation happens in container builds.

## Verify

- Server health:

```bash
curl -fsS http://localhost:8080/health
```

- Metrics:

```bash
curl -fsS http://localhost:9100/metrics | head
```

- Token list (uses local descriptor):

```bash
PLOY_CONFIG_HOME="$PWD/local/cli" ./dist/ploy cluster token list
```

## Stop / Clean

```bash
docker compose -f local/docker-compose.yml down -v
```

For a full reset including DB recreation:

```bash
./scripts/local-docker.sh --drop-db
```

## Troubleshooting

- `PLOY_LOCAL_PG_DSN` missing:
  - Set it before running scripts.
- DB unreachable:
  - Verify host PostgreSQL is running.
  - Verify DSN host is reachable from containers (`host.containers.internal` on macOS).
- Missing binaries:
  - Ensure `dist/ployd-linux` and `dist/ployd-node-linux` exist after `make build`.
- Node cannot run containers:
  - Ensure socket mount path is valid:
    - Docker script default: `/var/run/docker.sock`
    - Podman script default: `/run/user/$UID/podman/podman.sock`
- Logs:
  - `docker compose -f local/docker-compose.yml logs -f server`
  - `docker compose -f local/docker-compose.yml logs -f node`
  - `docker compose -f local/docker-compose.yml logs -f garage`

## Related

- `README.md`
- `docs/envs/README.md`
- `docs/how-to/token-management.md`
