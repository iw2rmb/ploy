# Deploy Full Local Stack To The VPS

`deploy/vps/run.sh` deploys the same Docker topology used by `deploy/runtime/run.sh`
to the fixed VPS target `s_v.v.kovalev@10.120.34.186` over SSH.

The script is intentionally simple:

- build binaries locally
- build or pull all required images locally
- copy a small runtime bundle to `/opt/ploy/current` on the VPS with `sudo`
- load the locally prepared Docker image archive on the VPS
- start the remote stack with `docker compose ... up --no-build`

No image builds happen on the VPS.

## Remote Requirements

The VPS must already have:

- Docker Engine with Compose v2
- `sudo` without an interactive password
- `psql`
- `pg_isready`
- `curl`
- `python3`

The PostgreSQL DSN must work both from the VPS host and from the `server`
container. Do not use `localhost` unless the container can resolve the same
database endpoint.

## Required Environment

On the local machine:

```bash
export PLOY_DB_DSN='postgres://ploy:ploy@10.120.34.186:5432/ploy?sslmode=disable'
```

Optional overrides:

```bash
export PLOY_SERVER_PORT=8080
export PLOY_REGISTRY_PORT=5000
export PLOY_CONTAINER_REGISTRY="127.0.0.1:${PLOY_REGISTRY_PORT}/ploy"
export PLOY_CA_CERTS='/absolute/path/to/ca-bundle.pem'
export CLUSTER_ID='local'
export NODE_ID='local1'
export AUTH_SECRET_PATH="$PWD/deploy/vps/auth-secret.txt"
```

`PLOY_CA_CERT` is accepted as an alias for `PLOY_CA_CERTS`.

## Deploy

From repo root:

```bash
./deploy/vps/run.sh
```

To force fresh local binaries and image rebuilds:

```bash
./deploy/vps/run.sh --clean
```

To reset the remote `ploy` database before the deploy:

```bash
./deploy/vps/run.sh --drop-db
```

## What The Script Does

- Reuses `dist/ployd-linux` and `dist/ployd-node-linux` when they already exist.
- Reuses local Docker images when the target tags already exist.
- Rebuilds both binaries and images when `--clean` is set.
- Keeps the `server` and `node` binaries in the uploaded `dist/*-linux` bundle and relies on the reused compose bind mounts instead of baking those binaries into the VPS runtime images.
- Uses `PLOY_CA_CERTS` exactly like `deploy/runtime/run.sh` on the local machine, and also installs that CA bundle on the VPS before Docker operations there.
- Uploads the runtime bundle under `/opt/ploy/current` with `sudo`.
- Creates `/private/tmp` on the VPS because the reused compose file bind-mounts that path for the node container.
- Starts `garage`, `garage-init`, `registry`, `gradle-build-cache`, and `server` first.
- Pushes the locally prepared workflow images into the VPS-local registry.
- Seeds admin and worker bearer tokens in PostgreSQL.
- Seeds the default node row and only then starts the `node` container.

## Verify

```bash
ssh s_v.v.kovalev@10.120.34.186 \
  'cd /opt/ploy/current && docker compose --project-name local --env-file deploy/vps/stack.env -f deploy/runtime/docker-compose.yml ps'
```

```bash
curl -fsS http://10.120.34.186:8080/health
```

```bash
curl -fsS http://10.120.34.186:5000/v2/
```

## Related

- [docs/how-to/deploy-locally.md](./deploy-locally.md)
- [docs/how-to/publish-migs.md](./publish-migs.md)
- [docs/envs/README.md](../envs/README.md)
