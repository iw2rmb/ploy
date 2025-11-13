# Deploy Ploy Locally (Docker)

This guide brings up a full local Ploy stack using Docker:
- PostgreSQL (db)
- Control plane `ployd` (server)
- One worker `ployd-node` (node) with access to your local Docker daemon

All files referenced live under `local/` and were added to the repo:
- `local/docker-compose.yml` — services and wiring
- `local/server/ployd.yaml` — server config (TLS/mTLS, metrics)
- `local/node/ployd-node.yaml` — node config (mTLS to server)
- `local/gen-certs.sh` — generates a CA + server/admin/node certs under `local/pki/`

See also: `docs/how-to/deploy-a-cluster.md` (server/node on VPS) and `docs/envs/README.md` (env vars).

## Prerequisites

- Docker Desktop 4.x (Compose v2). Docker daemon must be running.
- OpenSSL (`openssl` on PATH).
- Go 1.25+ to build local binaries.
- Open ports: 5432, 8443, 8444, 9100.
- macOS Apple Silicon: `local/docker-compose.yml` already pins `platform: linux/amd64` for the binaries.

## 1) Build Binaries

From the repo root:

```
make build
# Produces: dist/ploy, dist/ployd-linux, dist/ployd-node-linux
```

## 2) Generate Certificates (mTLS)

Use the helper script to create a cluster CA and sign server/admin/node certs:

```
./local/gen-certs.sh               # default node id: node-1
# or ./local/gen-certs.sh -n my-node-id
```

Outputs land in `local/pki/`:
- CA: `ca.crt` (key: `ca.key`)
- Server: `server.crt` (key: `server.key`)
- Admin (cli-admin): `admin.crt` (key: `admin.key`)
- Node (worker): `node.crt` (key: `node.key`)

## 3) Start The Stack

```
docker compose -f local/docker-compose.yml up -d
```

What you get:
- `db` (PostgreSQL 15) with `ploy` DB/user/password `ploy`.
- `server` exposing:
  - API `https://localhost:8443` (mTLS required)
  - Metrics `http://localhost:9100/metrics`
- `node` connecting to `server` using mTLS; Docker socket mounted for container work.

## 4) Verify

- Metrics (no auth):

```
curl -s http://localhost:9100/metrics | head
```

- API health (mTLS):

```
curl --cacert local/pki/ca.crt \
     --cert   local/pki/admin.crt \
     --key    local/pki/admin.key \
     https://localhost:8443/health
```

## 5) (Optional) Wire CLI To Local Server

Use an isolated config home and a simple descriptor that points the CLI at the local control plane over mTLS:

```
export PLOY_CONFIG_HOME="$PWD/local/cli"
mkdir -p "$PLOY_CONFIG_HOME/clusters"
cat > "$PLOY_CONFIG_HOME/clusters/local.json" <<JSON
{
  "cluster_id": "local",
  "address": "https://localhost:8443",
  "ca_path": "${PWD}/local/pki/ca.crt",
  "cert_path": "${PWD}/local/pki/admin.crt",
  "key_path": "${PWD}/local/pki/admin.key"
}
JSON
ln -sf local.json "$PLOY_CONFIG_HOME/clusters/default"

# Smoke a simple control-plane call (should return 200)
./dist/ploy config gitlab show || true
```

## 6) Stop / Clean

```
docker compose -f local/docker-compose.yml down -v
```

## Troubleshooting

- Ports in use: change the host ports in `local/docker-compose.yml`.
- Missing binaries: re-run `make build` and ensure `dist/ployd-linux` and `dist/ployd-node-linux` exist.
- Docker socket permission: the `node` service mounts `/var/run/docker.sock`; ensure Docker Desktop is running.
- Logs:
  - `docker compose -f local/docker-compose.yml logs -f server`
  - `docker compose -f local/docker-compose.yml logs -f node`
- Apple Silicon: `platform: linux/amd64` is set; if you rebuild binaries for arm64, remove the platform pin.

---

This local setup is for development only. For VPS deployment, use `dist/ploy server deploy` and `dist/ploy node add` as documented in `docs/how-to/deploy-a-cluster.md`.

