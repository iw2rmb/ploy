# Deploy Ploy Locally (Docker)

This guide brings up a full local Ploy stack using Docker:
- PostgreSQL (db)
- Control plane `ployd` (server)
- One worker `ployd-node` (node) with access to your local Docker daemon

All files referenced live under `local/` and were added to the repo:
- `local/docker-compose.yml` — services and wiring
- `local/server/ployd.yaml` — server config (bearer token authentication, metrics)
- `local/node/ployd-node.yaml` — node config (connects to server via HTTPS)

**Note**: As of the bearer token authentication migration, this deployment uses:
- **Bearer tokens** for CLI authentication (instead of mTLS client certificates)
- **Bootstrap tokens** for node provisioning
- **Plain HTTP** for ployd (HTTPS termination expected at load balancer in production)

See also: `docs/how-to/deploy-a-cluster.md` (server/node on VPS) and `docs/envs/README.md` (env vars).

## Prerequisites

- Docker Desktop 4.x (Compose v2). Docker daemon must be running.
- Go 1.25+ to build local binaries.
- Open ports: 5432, 8080, 8444, 9100.
- macOS Apple Silicon: `local/docker-compose.yml` already pins `platform: linux/amd64` for the binaries.

## 1) Build Binaries

From the repo root:

```
make build
# Produces: dist/ploy, dist/ployd-linux, dist/ployd-node-linux
```

## 2) Generate Authentication Secret

The server requires a secret for signing JWT tokens. Generate a secure random secret:

```bash
openssl rand -hex 32 > local/auth-secret.txt
```

This secret will be used by the server to sign and validate bearer tokens.

## 3) Start The Stack

Set the authentication secret and start the services:

```bash
export PLOY_AUTH_SECRET=$(cat local/auth-secret.txt)
docker compose -f local/docker-compose.yml up -d
```

What you get:
- `db` (PostgreSQL 15) with `ploy` DB/user/password `ploy`.
- `server` exposing:
  - API `http://localhost:8080` (plain HTTP, bearer token required)
  - Metrics `http://localhost:9100/metrics`
- `node` connecting to `server` using bootstrap token flow; Docker socket mounted for container work.

## 4) Create Initial Admin Token

After the server starts, you need to create an initial admin token. You can do this by directly connecting to the database or using the server's token generation endpoint.

For local development, you can generate a token manually:

```bash
# Connect to the running server container and generate a token
# This requires the server to expose a bootstrap endpoint or direct DB access
# For now, you'll need to use the CLI after it's configured with a token

# Alternative: Generate token directly via psql
docker exec -it ploy-db-1 psql -U ploy -d ploy -c "
INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
VALUES (
  encode(sha256('your-admin-token'::bytea), 'hex'),
  'initial-admin',
  'local',
  'cli-admin',
  'Initial admin token for local development',
  NOW(),
  NOW() + INTERVAL '365 days'
);"
```

**Note**: In production, use `ploy token create` after bootstrapping with the initial token.

## 5) Verify

- Metrics (no auth):

```bash
curl -s http://localhost:9100/metrics | head
```

- API health (bearer token):

```bash
# Using the admin token from step 4
curl -H "Authorization: Bearer your-admin-token" \
     http://localhost:8080/health
```

## 6) (Optional) Wire CLI To Local Server

Use an isolated config home and a simple descriptor that points the CLI at the local control plane with bearer token authentication:

```bash
export PLOY_CONFIG_HOME="$PWD/local/cli"
mkdir -p "$PLOY_CONFIG_HOME/clusters"
cat > "$PLOY_CONFIG_HOME/clusters/local.json" <<JSON
{
  "cluster_id": "local",
  "address": "http://localhost:8080",
  "token": "your-admin-token"
}
JSON
ln -sf local.json "$PLOY_CONFIG_HOME/clusters/default"

# Smoke a simple control-plane call (should return 200)
./dist/ploy config gitlab show || true
```

## 7) Stop / Clean

```
docker compose -f local/docker-compose.yml down -v
```

## Troubleshooting

- **Ports in use**: change the host ports in `local/docker-compose.yml` (default: 5432, 8080, 8444, 9100).
- **Missing binaries**: re-run `make build` and ensure `dist/ployd-linux` and `dist/ployd-node-linux` exist.
- **Docker socket permission**: the `node` service mounts `/var/run/docker.sock`; ensure Docker Desktop is running.
- **Authentication errors**: Ensure `PLOY_AUTH_SECRET` is set when starting the server and matches across restarts.
- **Token validation failures**: Check that the token is correctly formatted as a JWT and hasn't expired.
- **Node bootstrap failures**: Verify the bootstrap token hasn't expired (default: 15 minutes) and hasn't been used already.
- **Logs**:
  - Server: `docker compose -f local/docker-compose.yml logs -f server`
  - Node: `docker compose -f local/docker-compose.yml logs -f node`
  - Database: `docker compose -f local/docker-compose.yml logs -f db`
- **Apple Silicon**: `platform: linux/amd64` is set; if you rebuild binaries for arm64, remove the platform pin.
- **Token management**: Use `ploy token list`, `ploy token create`, and `ploy token revoke` commands after setting up the CLI descriptor.

## Token Management

After setting up the CLI (step 6), you can manage tokens:

```bash
# List all tokens
./dist/ploy token list

# Create a new token for CI/CD
./dist/ploy token create --role control-plane --expires 90d --description "CI/CD pipeline"

# Revoke a token
./dist/ploy token revoke <token-id>
```

See `docs/how-to/token-management.md` for detailed token management guide.

---

This local setup is for development only. For VPS deployment, use `dist/ploy server deploy` and `dist/ploy node add` as documented in `docs/how-to/deploy-a-cluster.md`.

