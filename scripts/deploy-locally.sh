#!/usr/bin/env bash
set -euo pipefail

# End-to-end local deployment for the Docker-based Ploy stack.
# This script automates the steps from docs/how-to/deploy-locally.md:
# - make build
# - generate auth secret (if missing)
# - start docker-compose stack
# - wait for object store bootstrap, db and server health
# - generate JWT admin + worker tokens and insert into api_tokens
# - seed local node record
# - provision worker bearer token into node container
# - wire local CLI descriptor

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f local/docker-compose.yml}"
CLUSTER_ID="${CLUSTER_ID:-local}"
# Node IDs must be NanoID(6) strings.
NODE_ID="${NODE_ID:-local1}"
AUTH_SECRET_PATH="${AUTH_SECRET_PATH:-local/auth-secret.txt}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$ROOT_DIR/local/cli}"
PLOY_EXTRA_CA_CERTS_PATH="${PLOY_EXTRA_CA_CERTS_PATH:-}"

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

build_local_images() {
  local extra_ca_path="$1"
  local platform="linux/amd64"

  if ! docker buildx version >/dev/null 2>&1; then
    echo "error: docker buildx is required (missing 'docker buildx')" >&2
    exit 1
  fi

  if [[ -n "$extra_ca_path" ]]; then
    if [[ ! -f "$extra_ca_path" ]]; then
      echo "error: PLOY_EXTRA_CA_CERTS_PATH does not exist: $extra_ca_path" >&2
      exit 1
    fi
    log "Building local images with extra CA certs (platform=$platform)..."
    docker buildx build --platform "$platform" --load \
      -f docker/server/Dockerfile -t ploy-server:local \
      --secret "id=ploy_extra_ca,src=$extra_ca_path" \
      .
    docker buildx build --platform "$platform" --load \
      -f docker/node/Dockerfile -t ploy-node:local \
      --secret "id=ploy_extra_ca,src=$extra_ca_path" \
      .
  else
    log "Building local images (platform=$platform)..."
    docker buildx build --platform "$platform" --load \
      -f docker/server/Dockerfile -t ploy-server:local \
      .
    docker buildx build --platform "$platform" --load \
      -f docker/node/Dockerfile -t ploy-node:local \
      .
  fi

  docker buildx build --platform "$platform" --load \
    -f docker/db/Dockerfile -t ploy-db:local \
    .
}

wait_for_garage_bootstrap() {
  local garage_cid garage_init_cid garage_health init_state init_exit

  garage_cid="$($COMPOSE_CMD ps -q garage)"
  garage_init_cid="$($COMPOSE_CMD ps -q garage-init)"
  if [[ -z "$garage_cid" || -z "$garage_init_cid" ]]; then
    echo "error: could not resolve garage container IDs" >&2
    $COMPOSE_CMD ps || true
    exit 1
  fi

  log "Waiting for Garage health and bootstrap completion..."
  for i in {1..90}; do
    garage_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$garage_cid" 2>/dev/null || true)"
    init_state="$(docker inspect -f '{{.State.Status}}' "$garage_init_cid" 2>/dev/null || true)"
    init_exit="$(docker inspect -f '{{.State.ExitCode}}' "$garage_init_cid" 2>/dev/null || true)"

    if [[ "$garage_health" == "healthy" && "$init_state" == "exited" && "$init_exit" == "0" ]]; then
      return 0
    fi

    if [[ "$init_state" == "exited" && "$init_exit" != "0" ]]; then
      echo "error: garage-init failed with exit code ${init_exit}" >&2
      $COMPOSE_CMD ps || true
      $COMPOSE_CMD logs garage garage-init || true
      exit 1
    fi

    sleep 1
  done

  echo "error: garage bootstrap did not complete in time" >&2
  $COMPOSE_CMD ps || true
  $COMPOSE_CMD logs garage garage-init || true
  exit 1
}

generate_tokens() {
  # Prints shell assignments for ADMIN_TOKEN*, WORKER_TOKEN* using PLOY_AUTH_SECRET.
  "$PYTHON_BIN" <<'PY'
import os, base64, json, hmac, hashlib, secrets, time

secret = os.environ["PLOY_AUTH_SECRET"]
cluster_id = os.environ.get("CLUSTER_ID", "local")

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

def gen_token(role: str):
    now = int(time.time())
    exp = now + 365*24*60*60
    header = {"alg": "HS256", "typ": "JWT"}
    jti = secrets.token_urlsafe(16)
    payload = {
        "cluster_id": cluster_id,
        "role": role,
        "token_type": "api",
        "iat": now,
        "exp": exp,
        "jti": jti,
    }
    header_b64 = b64url(json.dumps(header, separators=(",", ":")).encode("utf-8"))
    payload_b64 = b64url(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
    unsigned = f"{header_b64}.{payload_b64}"
    sig = hmac.new(secret.encode("utf-8"), unsigned.encode("utf-8"), hashlib.sha256).digest()
    token = unsigned + "." + b64url(sig)
    token_hash = hashlib.sha256(token.encode("utf-8")).hexdigest()
    return token, jti, token_hash

admin_token, admin_id, admin_hash = gen_token("cli-admin")
worker_token, worker_id, worker_hash = gen_token("worker")

print(f"ADMIN_TOKEN={admin_token}")
print(f"ADMIN_TOKEN_ID={admin_id}")
print(f"ADMIN_TOKEN_HASH={admin_hash}")
print(f"WORKER_TOKEN={worker_token}")
print(f"WORKER_TOKEN_ID={worker_id}")
print(f"WORKER_TOKEN_HASH={worker_hash}")
PY
}

main() {
  log "Checking prerequisites..."
  need docker
  need "$PYTHON_BIN"
  need openssl
  need make
  need curl

  log "Building CLI/binaries (make build)..."
  make build

  if [[ ! -f "$AUTH_SECRET_PATH" ]]; then
    log "Generating auth secret at $AUTH_SECRET_PATH..."
    mkdir -p "$(dirname "$AUTH_SECRET_PATH")"
    openssl rand -hex 32 > "$AUTH_SECRET_PATH"
  fi

  export PLOY_AUTH_SECRET
  PLOY_AUTH_SECRET="$(cat "$AUTH_SECRET_PATH")"
  export CLUSTER_ID

  build_local_images "$PLOY_EXTRA_CA_CERTS_PATH"

  log "Starting local docker stack with: $COMPOSE_CMD up -d --no-build"
  $COMPOSE_CMD up -d --no-build

  wait_for_garage_bootstrap

  log "Waiting for database to be ready..."
  for i in {1..60}; do
    if $COMPOSE_CMD exec -T db pg_isready -U ploy -d ploy >/dev/null 2>&1; then
      break
    fi
    sleep 1
    if [[ $i -eq 60 ]]; then
      echo "error: database did not become ready in time" >&2
      exit 1
    fi
  done

  log "Waiting for server health on http://localhost:8080/health..."
  for i in {1..60}; do
    if curl -fsS http://localhost:8080/health >/dev/null 2>&1; then
      break
    fi
    sleep 1
    if [[ $i -eq 60 ]]; then
      echo "error: server did not become healthy in time" >&2
      exit 1
    fi
  done

  log "Generating admin and worker JWT tokens..."
  # shellcheck disable=SC2046
  eval "$(generate_tokens)"

  log "Inserting admin token into api_tokens..."
  $COMPOSE_CMD exec -T db psql -U ploy -d ploy -v ON_ERROR_STOP=1 -c "\
    SET search_path TO ploy, public; \
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at) \
    VALUES ( \
      '${ADMIN_TOKEN_HASH}', \
      '${ADMIN_TOKEN_ID}', \
      '${CLUSTER_ID}', \
      'cli-admin', \
      'Initial admin token for local development', \
      NOW(), \
      NOW() + INTERVAL '365 days' \
    ) \
    ON CONFLICT (token_hash) DO NOTHING;"

  log "Inserting worker token into api_tokens..."
  $COMPOSE_CMD exec -T db psql -U ploy -d ploy -v ON_ERROR_STOP=1 -c "\
    SET search_path TO ploy, public; \
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at) \
    VALUES ( \
      '${WORKER_TOKEN_HASH}', \
      '${WORKER_TOKEN_ID}', \
      '${CLUSTER_ID}', \
      'worker', \
      'Local worker token for node ${NODE_ID}', \
      NOW(), \
      NOW() + INTERVAL '365 days' \
    ) \
    ON CONFLICT (token_hash) DO NOTHING;"

  log "Provisioning worker bearer token into node container..."
  NODE_CONTAINER_ID="$($COMPOSE_CMD ps -q node)"
  if [[ -z "$NODE_CONTAINER_ID" ]]; then
    echo "error: could not resolve node container id" >&2
    exit 1
  fi
  TMP_TOKEN_FILE="$(mktemp)"
  printf '%s' "$WORKER_TOKEN" > "$TMP_TOKEN_FILE"
  chmod 600 "$TMP_TOKEN_FILE"
  docker cp "$TMP_TOKEN_FILE" "$NODE_CONTAINER_ID:/etc/ploy/bearer-token"
  rm -f "$TMP_TOKEN_FILE"

  log "Seeding node record in ploy.nodes..."
  UUID="${NODE_ID}"
  NAME="${NODE_NAME:-local-node-0001}"
  IP="${NODE_IP:-127.0.0.1}"
  VERSION="${NODE_VERSION:-dev}"
  CONCURRENCY="${NODE_CONCURRENCY:-1}"
  log "Inserting node ${UUID} (${NAME} @ ${IP}) into ploy.nodes..."
  $COMPOSE_CMD exec -T db psql -U ploy -d ploy -v ON_ERROR_STOP=1 -c "\
    SET search_path TO ploy, public; \
    INSERT INTO nodes (id, name, ip_address, version, concurrency) \
    VALUES ('${UUID}', '${NAME}', '${IP}', '${VERSION}', ${CONCURRENCY}) \
    ON CONFLICT (id) DO NOTHING;"

  log "Restarting node service to pick up bearer token..."
  $COMPOSE_CMD restart node

  log "Wiring local CLI descriptor..."
  mkdir -p "$PLOY_CONFIG_HOME/clusters"
  cat > "$PLOY_CONFIG_HOME/clusters/local.json" <<JSON
{
  "cluster_id": "${CLUSTER_ID}",
  "address": "http://localhost:8080",
  "token": "${ADMIN_TOKEN}"
}
JSON
  ln -sf local.json "$PLOY_CONFIG_HOME/clusters/default"

  log "Configuring local Gradle Build Cache (scope=all)..."
  if [[ -x "./dist/ploy" ]]; then
    PLOY_CONFIG_HOME="$PLOY_CONFIG_HOME" ./dist/ploy config env set \
      --key PLOY_GRADLE_BUILD_CACHE_URL \
      --value "http://gradle-build-cache:5071/cache/" \
      --scope all

    PLOY_CONFIG_HOME="$PLOY_CONFIG_HOME" ./dist/ploy config env set \
      --key PLOY_GRADLE_BUILD_CACHE_PUSH \
      --value "true" \
      --scope all
  fi

  log "Smoke testing CLI cluster token list (optional)..."
  # NOTE: Token operations are now accessible only via `ploy cluster token`.
  if [[ -x "./dist/ploy" ]]; then
    PLOY_CONFIG_HOME="$PLOY_CONFIG_HOME" ./dist/ploy cluster token list || true
  fi

  if [[ -n "$PLOY_EXTRA_CA_CERTS_PATH" ]]; then
      PLOY_CONFIG_HOME=$PWD/local/cli ./dist/ploy config env set --key CA_CERTS_PEM_BUNDLE --file /Users/v.v.kovalev/ploy-mods/ca-certs.pem --scope gate
  fi

  log "Local Ploy cluster is up."
  log "Admin JWT (save securely):"
  echo "$ADMIN_TOKEN"
}

main "$@"
