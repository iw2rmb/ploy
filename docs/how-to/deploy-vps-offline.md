# Deploy Full Local Stack To An Offline VPS

This flow mirrors the `deploy/local` topology on a remote Linux VPS over SSH:

- `server`
- `node1`
- `node2`
- `garage`
- `garage-init`
- `registry`
- `gradle-build-cache`

It is intended for hosts that **cannot reach git remotes, public registries, or the internet at deploy time**. All images are built and bundled on the workstation, copied over SSH, then loaded and started remotely.

## What This Uses

- Local build machine:
  - `make build`
  - `docker buildx build --load`
  - `docker save`
  - `ssh` / `scp`
- Remote VPS:
  - Docker Engine with Compose v2
  - `psql`
  - `pg_isready`
  - `curl`
  - `python3`
  - PostgreSQL reachable from both the VPS host and the compose containers

Docker is still required on the VPS. The worker runtime and Build Gate remain Docker-based.

## Required Environment

On the local build machine:

```bash
export PLOY_VPS_DB_DSN='postgres://ploy:ploy@10.0.0.10:5432/ploy?sslmode=disable'
```

This DSN must work:

- from the VPS host for `psql`/`pg_isready`
- from the `server` container as `PLOY_POSTGRES_DSN`

Optional overrides:

```bash
export PLOY_VPS_CLUSTER_ID='local'
export PLOY_VPS_WORKDIR_ROOT='/var/tmp/ploy-vps'
export PLOY_SERVER_PORT=8080
export PLOY_REGISTRY_PORT=5000
export PLOY_CONTAINER_REGISTRY="127.0.0.1:${PLOY_REGISTRY_PORT}/ploy"
export PLOY_CA_CERTS='/absolute/path/to/ca-bundle.pem'  # optional: custom CA for docker.io registry trust
export PLOY_SKIP_BUILD=1
```

## Deploy

From repo root:

```bash
./deploy/vps/redeploy.sh \
  --address 203.0.113.10 \
  --user root \
  --identity ~/.ssh/id_rsa
```

When local disk space is tight, enable low-disk mode:

```bash
./deploy/vps/redeploy.sh \
  --address 203.0.113.10 \
  --user root \
  --identity ~/.ssh/id_rsa \
  --low-disk
```

To reset the `ploy` database during redeploy:

```bash
./deploy/vps/redeploy.sh \
  --address 203.0.113.10 \
  --user root \
  --identity ~/.ssh/id_rsa \
  --drop-db
```

What the script does:

- Builds local binaries with `make build` unless `PLOY_SKIP_BUILD` is true-like.
- In `--low-disk` mode, reuses `dist/ploy` when it already exists locally.
- Builds runtime images locally:
  - `ploy-server:vps`
  - `ploy-node:vps`
  - `ploy-garage-init:vps`
- Builds workflow images locally under `${PLOY_CONTAINER_REGISTRY}`:
  - mods images from `deploy/images/mig/*` and `deploy/images/migs/*`
  - Gradle gate images
  - mirrored base images required by `gates/stacks.yaml`
- Pulls service images needed by the compose stack.
- In `--low-disk` mode, reuses any already-present local Docker images for those tags instead of rebuilding or repulling them.
- Generates one admin token and two worker tokens locally.
- Writes a bundle with compose/config/token files plus `docker save` output.
- In `--low-disk` mode, streams the bundle and `docker save` payload over SSH instead of writing the large archive twice on the workstation.
- Uploads that bundle over SSH.
- Runs the remote apply script, which:
  - ensures the `ploy` database exists
  - loads Docker images from the bundle
  - starts the compose stack
  - seeds Garage/registry readiness
  - pushes bundled workflow images into the remote local registry
  - inserts admin/worker tokens into `api_tokens`
  - seeds node rows for `node1` and `node2`
  - configures remote Gradle cache env through the server API

## Local CLI Descriptor

After a successful deploy, the script writes a local cluster descriptor under:

```bash
deploy/vps/cli
```

Override with:

```bash
export PLOY_CONFIG_HOME=/path/to/config-home
```

Then use the CLI against the remote stack:

```bash
PLOY_CONFIG_HOME="$PWD/deploy/vps/cli" ./dist/ploy cluster token list
```

## Runtime Layout

Two node containers are started:

- `node1` with `node_id=local1`
- `node2` with `node_id=local2`

Their host-visible workspace roots default to:

- `/var/tmp/ploy-vps/node1`
- `/var/tmp/ploy-vps/node2`

These paths are bind-mounted at the same absolute locations inside the node containers so host Docker can mount job workspaces correctly.

## Verify On The VPS

```bash
ssh -i ~/.ssh/id_rsa root@203.0.113.10 \
  'cd /opt/ploy-vps/current && docker compose --project-name ploy-vps --env-file stack.env -f docker-compose.yml ps'
```

```bash
curl -fsS http://203.0.113.10:8080/health
```

```bash
curl -fsS http://203.0.113.10:5000/v2/
```

## Related

- [docs/how-to/deploy-locally.md](./deploy-locally.md)
- [docs/how-to/publish-migs.md](./publish-migs.md)
- [docs/envs/README.md](../envs/README.md)
