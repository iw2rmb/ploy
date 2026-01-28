# Deploy Ploy Locally (Docker)

This guide brings up a full local Ploy stack using Docker:
- PostgreSQL (db)
- Control plane `ployd` (server)
- One worker `ployd-node` (node) with access to your local Docker daemon

All files referenced live under `local/` and were added to the repo:
- `local/docker-compose.yml` — services and wiring
- `local/server/ployd.yaml` — server config (bearer token authentication, metrics)
- `local/node/ployd-node.yaml` — node config (connects to server via HTTP with bearer tokens)

**Note**: As of the bearer token authentication migration, this deployment uses:
- **Bearer tokens** for CLI authentication (instead of mTLS client certificates)
- **Bearer tokens** for worker (node) authentication against the control plane
- **Plain HTTP** for ployd in this Docker stack

## Prerequisites

- Docker Desktop 4.x (Compose v2). Docker daemon must be running.
- Go 1.25+ to build local binaries.
- Open ports: 5432, 8080, 8444, 9100.
- macOS Apple Silicon: `local/docker-compose.yml` already pins `platform: linux/amd64` for the binaries.
- (Optional) Corporate TLS MITM / custom registries: set `PLOY_EXTRA_CA_CERTS_PATH` to a PEM bundle to inject extra CAs into the `ployd` and `ployd-node` images during local builds.

## Quickstart (recommended)

Use the automation script from the repo root:

```bash
# Optional: inject extra CA certs into server/node images during build
export PLOY_EXTRA_CA_CERTS_PATH=/path/to/ca-bundle.pem

./scripts/deploy-locally.sh
```

This script builds images with Docker BuildKit and seeds the local DB with the initial tokens and node record.

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
- `node` calling the control plane using bearer token authentication; Docker socket mounted for container work.

## 4) Create Initial Admin Token

After the server starts, create an initial **JWT** admin token and register it in PostgreSQL.

1. Generate a JWT admin token (on the host, from the repo root):

```bash
# Reuse the secret from step 2
export PLOY_AUTH_SECRET="$(cat local/auth-secret.txt)"

python - <<'PY'
import os, base64, json, hmac, hashlib, secrets, time

secret = os.environ["PLOY_AUTH_SECRET"]
cluster_id = "local"
role = "cli-admin"
now = int(time.time())
exp = now + 365*24*60*60

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

header = {"alg": "HS256", "typ": "JWT"}
payload = {
    "cluster_id": cluster_id,
    "role": role,
    "token_type": "api",
    "iat": now,
    "exp": exp,
    "jti": secrets.token_urlsafe(16),
}

header_b64 = b64url(json.dumps(header, separators=(",", ":")).encode("utf-8"))
payload_b64 = b64url(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
unsigned = f"{header_b64}.{payload_b64}"
sig = hmac.new(secret.encode("utf-8"), unsigned.encode("utf-8"), hashlib.sha256).digest()
token = unsigned + "." + b64url(sig)

print("TOKEN=" + token)
print("TOKEN_ID=" + payload["jti"])
PY
```

2. Copy `TOKEN` and `TOKEN_ID` from the output, then compute the hash:

```bash
TOKEN="...value from previous step..."
TOKEN_ID="...value from previous step..."

TOKEN_HASH="$(printf '%s' "$TOKEN" | sha256sum | awk '{print $1}')"
```

3. Insert the token into PostgreSQL for cluster `local`:

```bash
docker exec -it ploy-db-1 psql -U ploy -d ploy -c "
INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
VALUES (
  '${TOKEN_HASH}',
  '${TOKEN_ID}',
  'local',
  'cli-admin',
  'Initial admin token for local development',
  NOW(),
  NOW() + INTERVAL '365 days'
);"
```

Use the `TOKEN` value as the admin token in the following steps.

**Note**: In production (and once this token exists), use `ploy cluster token create` / `ploy cluster token revoke` for ongoing token management.

## 5) Verify

- Metrics (no auth):

```bash
curl -s http://localhost:9100/metrics | head
```

- API health (bearer token):

```bash
# Using the admin token from step 4
curl -H "Authorization: Bearer <JWT-from-step-4>" \
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
  "token": "<JWT-from-step-4>"
}
JSON
ln -sf local.json "$PLOY_CONFIG_HOME/clusters/default"

# Smoke a simple control-plane call (should return 200)
./dist/ploy config gitlab show || true
```

## 7) Gradle Build Cache (optional)

The local stack includes a Gradle Build Cache Node (`gradle-build-cache`)
that gate containers can use to share compiled outputs across runs.

For local development, the cache node is configured to allow **anonymous read/write**
access on the Docker network. Do not reuse this configuration in non-local environments.

The local stack seeds the node config from `local/gradle-build-cache/config.yaml` and forces
`anonymousLevel: readwrite` on every startup (persisted under the `gradle-build-cache-data`
volume at `/data/conf/config.yaml`).

Build the local Build Gate Gradle images (required for `java+gradle` gates when using the default image mapping):

The node resolves these images via the local Docker daemon. When the image already exists locally, the node will use it as-is; it only pulls from a registry when the image is missing.

```bash
docker build -t ploy-gate-gradle:jdk11 -f docker/gates/gradle/Dockerfile.jdk11 docker/gates/gradle
docker build -t ploy-gate-gradle:jdk17 -f docker/gates/gradle/Dockerfile.jdk17 docker/gates/gradle
```

`./scripts/deploy-locally.sh` configures gate + mod jobs to use the cache automatically.
If you started the stack manually, configure it via:

```bash
./dist/ploy config env set --key PLOY_GRADLE_BUILD_CACHE_URL \
  --value "http://gradle-build-cache:5071/cache/" \
  --scope all

./dist/ploy config env set --key PLOY_GRADLE_BUILD_CACHE_PUSH \
  --value "true" \
  --scope all
```

The cache node UI is available at `http://localhost:5071`.

Run the Gradle build cache E2E scenario:

```bash
bash tests/e2e/gradle/build-cache/run.sh
```

## 8) Stop / Clean

```
docker compose -f local/docker-compose.yml down -v
```

## Troubleshooting

- **Ports in use**: change the host ports in `local/docker-compose.yml` (default: 5432, 8080, 8444, 9100).
- **Missing binaries**: re-run `make build` and ensure `dist/ployd-linux` and `dist/ployd-node-linux` exist.
- **Docker socket permission**: the `node` service mounts `/var/run/docker.sock`; ensure Docker Desktop is running.
- **`apk add` failures / TLS errors during image build**: set `PLOY_EXTRA_CA_CERTS_PATH` to a PEM bundle and run `./scripts/deploy-locally.sh` (it builds `ploy-server:local` and `ploy-node:local` with the extra CA injected).
- **Authentication errors**: Ensure `PLOY_AUTH_SECRET` is set when starting the server and matches across restarts.
- **Token validation failures**: Check that the token is correctly formatted as a JWT and hasn't expired.
- **Logs**:
  - Server: `docker compose -f local/docker-compose.yml logs -f server`
  - Node: `docker compose -f local/docker-compose.yml logs -f node`
  - Database: `docker compose -f local/docker-compose.yml logs -f db`
- **Apple Silicon**: `platform: linux/amd64` is set; if you rebuild binaries for arm64, remove the platform pin.
- **Token management**: Use `ploy cluster token list`, `ploy cluster token create`, and `ploy cluster token revoke` commands after setting up the CLI descriptor.

## Token Management

After setting up the CLI (step 6), you can manage tokens:

```bash
# List all tokens
./dist/ploy cluster token list

# Create a new token for CI/CD
./dist/ploy cluster token create --role control-plane --expires 90 --description "CI/CD pipeline"

# Revoke a token
./dist/ploy cluster token revoke <token-id>
```

See `docs/how-to/token-management.md` for detailed token management guide.

---

This local Docker setup is the supported environment for development and tests.
