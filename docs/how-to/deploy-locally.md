# Deploy Ploy Locally (Docker)

This guide brings up the local Ploy stack using host PostgreSQL plus containers for:
- Garage object store (`garage`) and bootstrap (`garage-init`)
- Local OCI registry backed by Garage (`registry`)
- Control plane `ployd` (`server`)
- Worker `ployd-node` (`node`)
- Gradle cache (`gradle-build-cache`)

The local stack no longer runs a PostgreSQL container. The server uses your host PostgreSQL via `PLOY_DB_DSN`.

## Runtime Mode (GHCR Images + Auto-Update)

For workstation/runtime deployments that should not build local binaries/images, use:

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable'
export PLOY_CA_CERTS='/path/to/ca-bundle.pem'               # optional: used for docker daemon trust + runtime container trust
./deploy/runtime/run.sh
export PLOY_CONFIG_HOME="$PWD/deploy/runtime/cli"
```

`deploy/runtime/run.sh`:
- Pulls runtime images (`docker compose pull`) before each start by default.
- Starts the runtime stack from `deploy/runtime/docker-compose.yml`.
- Uses GHCR images pinned by `PLOY_VERSION` (defaults to `./VERSION`, semver):
  - `ghcr.io/iw2rmb/ploy-server:${PLOY_VERSION}`
  - `ghcr.io/iw2rmb/ploy-node:${PLOY_VERSION}`
  - `ghcr.io/iw2rmb/ploy-garage-init:${PLOY_VERSION}`
- Injects runtime CA bundle locally (when `PLOY_CA_CERTS` is set) without baking certs into images.
- Seeds `CA_CERTS_PEM_BUNDLE` global env from `PLOY_CA_CERTS` so mig/build-gate containers also receive the same CA bundle at runtime.

## Prerequisites

- Go toolchain `go1.25.8`. `make` targets fail fast on other versions; run with
  `GOTOOLCHAIN=go1.25.8` when your host default differs.
- Local PostgreSQL running and reachable from containers.
- `psql` and `pg_isready` available on the host.
- Docker Desktop / Docker Engine with Compose v2.

## Quickstart (Docker)

From repo root:

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable'
export PLOY_CA_CERTS='/path/to/ca-bundle.pem'  # optional: docker registry + runtime container trust
export PLOY_SERVER_PORT=18080   # optional; default 8080
export PLOY_REGISTRY_PORT=5000  # optional; default 5000
export PLOY_CONTAINER_REGISTRY="127.0.0.1:${PLOY_REGISTRY_PORT}/ploy"
export PLOY_S3_URL='http://localhost:3900'
export PLOY_S3_ACCESS_KEY='...'
export PLOY_S3_SECRET_KEY='...'
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
  - object bucket: `ploy` (logs/diffs/artifacts)
  - registry bucket: `ploy-registry` (OCI image blobs/manifests)

## Script Flags

`./deploy/local/run.sh` supports:

```bash
--drop-db   # drop + recreate "ploy" before deploy
--ployd     # refresh/deploy server only
--nodes     # refresh/deploy node (+ required server dependency)
```

No flags means full deploy (server + node + garage + registry services).

## Binary Mount Model

- Host binaries are mounted into containers:
  - `dist/ployd-linux -> /usr/local/bin/ployd`
  - `dist/ployd-node-linux -> /usr/local/bin/ployd-node`
- Runtime images are built from:
  - `deploy/images/server/Dockerfile`
  - `deploy/images/node/Dockerfile`
  - `deploy/local/garage/Dockerfile` (bootstrap helper image with `/garage` + shell)
- Runtime containers execute host-built binaries mounted from `dist/`.
- Core Dockerfiles are used for local runtime image builds.

## Verify

- Server health:

```bash
curl -fsS "http://127.0.0.1:${PLOY_SERVER_PORT:-8080}/health"
```

- Metrics:

```bash
curl -fsS http://localhost:9100/metrics | head
```

- Registry API health:

```bash
curl -fsS "http://localhost:${PLOY_REGISTRY_PORT:-5000}/v2/"
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
  - Verify the DSN host is reachable from containers.
  - For loopback DSNs (`localhost`, `127.0.0.1`, `::1`), local deploy rewrites the container DSN host to `host.docker.internal`.
- Docker Hub TLS verification fails during image build:
  - Set `PLOY_CA_CERTS` to a PEM bundle path trusted in your environment.
  - Re-run `./deploy/local/run.sh`; it installs this CA for Docker registry trust (Colima/Linux automation).
- Host `localhost` API path is intercepted/proxied:
  - `run.sh` health checks use Docker container health state instead of host `curl`.
  - Global env bootstrap (`PLOY_GRADLE_BUILD_CACHE_*`) falls back to server-container local API calls when host CLI HTTP calls fail.
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
  - `docker compose -f deploy/local/docker-compose.yml logs -f registry`

## Related

- `README.md`
- `docs/envs/README.md`
- `docs/how-to/token-management.md`
