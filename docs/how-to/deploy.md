# Deploy Ploy Cluster

This guide uses a single deployment path: `ploy cluster deploy`.

`ploy cluster deploy` extracts embedded runtime assets and deploys the Docker stack
(server, node, object store, registry, gradle cache) for the selected cluster.

## Prerequisites

- Docker Engine/Desktop with Compose v2
- Host PostgreSQL reachable from containers
- `psql` and `pg_isready`
- `ploy` CLI built/installed

## Quickstart

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable'
export PLOY_OBJECTSTORE_ENDPOINT='http://localhost:3900'
export PLOY_OBJECTSTORE_ACCESS_KEY='...'
export PLOY_OBJECTSTORE_SECRET_KEY='...'
export PLOY_VERSION='v0.1.0'                    # optional; defaults to CLI version when semver

ploy cluster deploy --cluster local

# If you need custom CA certificates, register them via typed config:
# ploy config ca set --file /path/to/ca-bundle.pem
```

Notes:
- `PLOY_CONFIG_HOME` defaults to `~/.config/ploy`.
- Auth descriptor is written under `~/.config/ploy/<cluster>/auth.json`.
- Worker token is persisted at `~/.config/ploy/<cluster>/bearer-token`.

## Command

```bash
ploy cluster deploy [--drop-db] [--ployd] [--nodes] [--no-pull] [--cluster <id>] [cluster]
```

Flags:
- `--drop-db`: drop and recreate `ploy` database before deploy
- `--ployd`: refresh/deploy server only
- `--nodes`: refresh/deploy node (includes required server dependency)
- `--no-pull`: skip image pull before `up`
- `--cluster <id>` or positional `cluster`: target cluster id

No flags means full stack deploy.

## Verify

```bash
curl -fsS "http://127.0.0.1:${PLOY_SERVER_PORT:-8080}/health"
curl -fsS http://localhost:9100/metrics | head
ploy cluster token list
```

## Stop / Reset

Stop all runtime containers managed by compose service labels:

```bash
docker ps -q --filter label=com.docker.compose.service=server | xargs -r docker stop
docker ps -q --filter label=com.docker.compose.service=node | xargs -r docker stop
docker ps -q --filter label=com.docker.compose.service=gradle-build-cache | xargs -r docker stop
```

Full reset with DB recreation:

```bash
ploy cluster deploy --drop-db --cluster local
```

## Existing DB Cleanup (No Recreate)

For existing databases, you can remove obsolete replay/mirroring data without
dropping the database:

```sql
DROP INDEX IF EXISTS jobs_cache_lookup_idx;
ALTER TABLE jobs DROP COLUMN IF EXISTS cache_key;
UPDATE jobs
SET meta = meta - 'cache_mirror'
WHERE meta ? 'cache_mirror';
```

This cleanup is optional for rows that never used replay metadata, but safe to
run repeatedly.

## Troubleshooting

- `PLOY_DB_DSN` missing/unreachable:
  - set DSN and verify PostgreSQL is reachable from containers.
- Port `8080` conflict:
  - set `PLOY_SERVER_PORT` (for example `18080`).
- TLS issues pulling images:
  - register the CA bundle via `ploy config ca set --file /path/to/ca-bundle.pem` and rerun `ploy cluster deploy`.
- Node cannot run containers:
  - verify `PLOY_CONTAINER_SOCKET_PATH` (default `/var/run/docker.sock`).
- Logs:
  - `docker logs -f "$(docker ps -q --filter label=com.docker.compose.service=server | head -n1)"`
  - `docker logs -f "$(docker ps -q --filter label=com.docker.compose.service=node | head -n1)"`

## Related

- `README.md`
- `docs/envs/README.md`
- `docs/how-to/token-management.md`
