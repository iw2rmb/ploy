#!/usr/bin/env bash
set -euo pipefail

# End-to-end local deployment for the Podman-based Ploy stack.
# - Builds local host binaries (no Go container builds for ployd binaries)
# - Uses host PostgreSQL (no db service in compose)
# - Mounts host binaries into containers via volumes

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-podman-compose -f local/docker-compose.yml}"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-podman}"
CLUSTER_ID="${CLUSTER_ID:-local}"
NODE_ID="${NODE_ID:-local1}"
AUTH_SECRET_PATH="${AUTH_SECRET_PATH:-local/auth-secret.txt}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$ROOT_DIR/local/cli}"
PLOY_DB_DSN="${PLOY_DB_DSN:-}"
PLOY_CONTAINER_SOCKET_PATH="${PLOY_CONTAINER_SOCKET_PATH:-}"
PLOY_SERVER_PORT="${PLOY_SERVER_PORT:-8080}"
WORKER_TOKEN_PATH="${WORKER_TOKEN_PATH:-$ROOT_DIR/local/node/bearer-token}"

DROP_DB=0
REFRESH_PLOYD=0
REFRESH_NODES=0

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

usage() {
  cat <<'USAGE'
Usage: ./scripts/local-podman.sh [--drop-db] [--ployd] [--nodes]

Options:
  --drop-db  Drop and recreate the ploy database before deploy
  --ployd    Refresh/deploy server only
  --nodes    Refresh/deploy node (includes required server dependency)

Environment:
  PLOY_DB_DSN        PostgreSQL DSN used by host setup and server container
  PLOY_SERVER_PORT  Host port for server HTTP endpoint (default: 8080)
  PLOY_CONTAINER_SOCKET_PATH  Podman socket mounted to /var/run/docker.sock (auto-detected by default)
  WORKER_TOKEN_PATH       Host path mounted to /etc/ploy/bearer-token in node (default: local/node/bearer-token)
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --drop-db)
        DROP_DB=1
        ;;
      --ployd)
        REFRESH_PLOYD=1
        ;;
      --nodes)
        REFRESH_NODES=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "error: unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
  done
}

detect_socket_from_default_connection() {
  local uri path

  uri="$(podman system connection list 2>/dev/null | awk 'NR>1 && $4=="true" {print $2; exit}')"
  if [[ -z "$uri" ]]; then
    return 1
  fi

  path="$("$PYTHON_BIN" - "$uri" <<'PY'
import sys
from urllib.parse import urlsplit

uri = sys.argv[1].strip()
if not uri:
    raise SystemExit(1)
if "://" not in uri:
    print(uri)
    raise SystemExit(0)

u = urlsplit(uri)
if u.path:
    print(u.path)
PY
)"
  if [[ -z "$path" ]]; then
    return 1
  fi

  case "$path" in
    */run/podman/podman.sock)
      printf '/run/podman/podman.sock'
      ;;
    */run/user/*/podman/podman.sock)
      printf '%s' "$path"
      ;;
    *)
      return 1
      ;;
  esac
}

detect_socket_from_machine_mode() {
  local machine_name rootful

  machine_name="${PLOY_PODMAN_MACHINE_NAME:-podman-machine-default}"
  rootful="$(podman machine inspect "$machine_name" 2>/dev/null | "$PYTHON_BIN" <<'PY'
import json
import sys

try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)

if not isinstance(data, list) or not data:
    raise SystemExit(1)

print("true" if data[0].get("Rootful") else "false")
PY
)"

  case "$rootful" in
    true)
      printf '/run/podman/podman.sock'
      ;;
    false)
      printf '/run/user/%s/podman/podman.sock' "$UID"
      ;;
    *)
      return 1
      ;;
  esac
}

resolve_container_socket_path() {
  local detected

  if [[ -n "$PLOY_CONTAINER_SOCKET_PATH" ]]; then
    return
  fi

  detected="$(detect_socket_from_default_connection || true)"
  if [[ -z "$detected" ]]; then
    detected="$(detect_socket_from_machine_mode || true)"
  fi
  if [[ -z "$detected" ]]; then
    detected="/run/user/${UID}/podman/podman.sock"
  fi

  PLOY_CONTAINER_SOCKET_PATH="$detected"
}

derive_admin_pg_dsn() {
  "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import urlsplit, urlunsplit

dsn = os.environ["PLOY_DB_DSN"].strip()
if not dsn:
    raise SystemExit("error: PLOY_DB_DSN is required")

if "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)")

u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")

if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")

admin = urlunsplit((u.scheme, u.netloc, "/postgres", u.query, u.fragment))
print(admin)
PY
}

normalize_container_pg_dsn() {
  PLOY_DB_DSN="$PLOY_DB_DSN" \
  USER="${USER:-}" \
  "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import quote, urlsplit, urlunsplit

def parse_dsn(name: str):
    dsn = os.environ[name].strip()
    if not dsn:
        raise SystemExit(f"error: {name} is required")
    if "://" not in dsn:
        raise SystemExit(f"error: {name} must be a URL DSN (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)")

    u = urlsplit(dsn)
    if u.scheme not in ("postgres", "postgresql"):
        raise SystemExit(f"error: {name} must use postgres:// or postgresql://")
    if u.path.strip("/") != "ploy":
        raise SystemExit(f"error: {name} must target database ploy")
    return u

dsn_u = parse_dsn("PLOY_DB_DSN")

username = dsn_u.username or os.environ.get("USER", "").strip()
if not username:
    raise SystemExit("error: unable to infer postgres username; include username in PLOY_DB_DSN or set USER")

password = dsn_u.password

host = dsn_u.hostname or ""
if ":" in host and not host.startswith("["):
    host = f"[{host}]"

userinfo = quote(username, safe="")
if password is not None:
    userinfo += ":" + quote(password, safe="")

port = f":{dsn_u.port}" if dsn_u.port else ""
netloc = f"{userinfo}@{host}{port}"
normalized = urlunsplit((dsn_u.scheme, netloc, dsn_u.path, dsn_u.query, dsn_u.fragment))
print(normalized)
PY
}

validate_container_pg_dsn() {
  PLOY_DB_DSN="$PLOY_DB_DSN" "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import urlsplit

dsn = os.environ["PLOY_DB_DSN"].strip()
if not dsn:
    raise SystemExit("error: PLOY_DB_DSN is required")
if "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)")

u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")
if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")

host = (u.hostname or "").lower()
if not host:
    raise SystemExit("error: PLOY_DB_DSN must include a TCP hostname reachable from containers")
if host in ("localhost", "127.0.0.1", "::1"):
    raise SystemExit("error: PLOY_DB_DSN must not use localhost/loopback; use host.containers.internal or another host reachable from containers")
PY
}

wait_for_postgres() {
  local admin_dsn="$1"
  log "Waiting for local PostgreSQL to be ready..."
  for i in {1..60}; do
    if pg_isready -d "$admin_dsn" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "error: local PostgreSQL did not become ready in time" >&2
  exit 1
}

drop_and_recreate_ploy_db() {
  local admin_dsn="$1"

  log "Dropping and recreating database 'ploy'..."
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'ploy' AND pid <> pg_backend_pid();" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "DROP DATABASE IF EXISTS ploy;" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "CREATE DATABASE ploy;" >/dev/null
}

ensure_ploy_db_exists() {
  local admin_dsn="$1"
  local exists

  exists="$(psql "$admin_dsn" -v ON_ERROR_STOP=1 -qXAt -c "SELECT 1 FROM pg_database WHERE datname = 'ploy' LIMIT 1;")"
  if [[ "$exists" == "1" ]]; then
    log "Database 'ploy' already exists."
    return
  fi

  log "Creating database 'ploy'..."
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "CREATE DATABASE ploy;" >/dev/null
}

build_runtime_images() {
  local -a services=("$@")

  if [[ ${#services[@]} -eq 0 ]]; then
    return
  fi

  log "Building runtime images: ${services[*]}"
  $COMPOSE_CMD build "${services[@]}"
}

remove_stale_service_containers() {
  local -a services=("$@")
  local idx service name include_node include_server
  local -a all_services=("${services[@]}")

  include_server=1
  for service in "${all_services[@]}"; do
    if [[ "$service" == "server" ]]; then
      include_server=0
      break
    fi
  done
  if [[ $include_server -eq 1 ]]; then
    all_services+=("server")
  fi

  include_node=1
  for service in "${all_services[@]}"; do
    if [[ "$service" == "node" ]]; then
      include_node=0
      break
    fi
  done
  if [[ $include_node -eq 1 ]]; then
    all_services+=("node")
  fi

  if [[ ${#all_services[@]} -eq 0 ]]; then
    return
  fi

  log "Removing stale service containers: ${all_services[*]}"
  for ((idx=${#all_services[@]} - 1; idx>=0; idx--)); do
    service="${all_services[$idx]}"
    name="local_${service}_1"
    if $CONTAINER_ENGINE inspect "$name" >/dev/null 2>&1; then
      $CONTAINER_ENGINE rm -f "$name" >/dev/null 2>&1 || true
    fi
  done
}

compose_container_id() {
  local service="$1"
  local cid

  cid="$($CONTAINER_ENGINE ps -aq \
    --filter "label=io.podman.compose.service=${service}" \
    --filter "label=com.docker.compose.project.working_dir=${ROOT_DIR}/local" | head -n1)"
  if [[ -z "$cid" ]]; then
    cid="$($CONTAINER_ENGINE ps -aq --filter "label=io.podman.compose.service=${service}" | head -n1)"
  fi
  if [[ -z "$cid" ]]; then
    cid="$($CONTAINER_ENGINE ps -aq --filter "name=^local_${service}_1$" | head -n1)"
  fi

  printf '%s' "$cid"
}

dump_compose_service_logs() {
  local service="$1"
  $COMPOSE_CMD logs "$service" || true
}

wait_for_garage_bootstrap() {
  local garage_cid garage_init_cid garage_health init_state init_exit

  garage_cid="$(compose_container_id garage)"
  garage_init_cid="$(compose_container_id garage-init)"
  if [[ -z "$garage_cid" || -z "$garage_init_cid" ]]; then
    echo "error: could not resolve garage container IDs" >&2
    $COMPOSE_CMD ps || true
    exit 1
  fi

  log "Waiting for Garage health and bootstrap completion..."
  for i in {1..90}; do
    garage_health="$($CONTAINER_ENGINE inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$garage_cid" 2>/dev/null || true)"
    init_state="$($CONTAINER_ENGINE inspect -f '{{.State.Status}}' "$garage_init_cid" 2>/dev/null || true)"
    init_exit="$($CONTAINER_ENGINE inspect -f '{{.State.ExitCode}}' "$garage_init_cid" 2>/dev/null || true)"

    if [[ "$garage_health" == "healthy" && "$init_state" == "exited" && "$init_exit" == "0" ]]; then
      return 0
    fi

    if [[ "$init_state" == "exited" && "$init_exit" != "0" ]]; then
      echo "error: garage-init failed with exit code ${init_exit}" >&2
      $COMPOSE_CMD ps || true
      dump_compose_service_logs garage
      dump_compose_service_logs garage-init
      exit 1
    fi

    sleep 1
  done

  echo "error: garage bootstrap did not complete in time" >&2
  $COMPOSE_CMD ps || true
  dump_compose_service_logs garage
  dump_compose_service_logs garage-init
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

wait_for_server_health() {
  local server_url="http://localhost:${PLOY_SERVER_PORT}"
  log "Waiting for server health on ${server_url}/health..."
  for i in {1..60}; do
    if curl -fsS "${server_url}/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "error: server did not become healthy in time" >&2
  exit 1
}

seed_tokens() {
  log "Inserting admin token into api_tokens..."
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES (
      '${ADMIN_TOKEN_HASH}',
      '${ADMIN_TOKEN_ID}',
      '${CLUSTER_ID}',
      'cli-admin',
      'Initial admin token for local development',
      NOW(),
      NOW() + INTERVAL '365 days'
    )
    ON CONFLICT (token_hash) DO NOTHING;"

  log "Inserting worker token into api_tokens..."
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES (
      '${WORKER_TOKEN_HASH}',
      '${WORKER_TOKEN_ID}',
      '${CLUSTER_ID}',
      'worker',
      'Local worker token for node ${NODE_ID}',
      NOW(),
      NOW() + INTERVAL '365 days'
    )
    ON CONFLICT (token_hash) DO NOTHING;"
}

provision_worker_token_into_node() {
  local token_dir tmp_token_file

  log "Writing worker bearer token to ${WORKER_TOKEN_PATH}..."
  token_dir="$(dirname "$WORKER_TOKEN_PATH")"
  mkdir -p "$token_dir"
  if [[ -d "$WORKER_TOKEN_PATH" ]]; then
    rm -rf "$WORKER_TOKEN_PATH"
  fi

  tmp_token_file="$(mktemp)"
  printf '%s' "$WORKER_TOKEN" > "$tmp_token_file"
  chmod 600 "$tmp_token_file"
  mv "$tmp_token_file" "$WORKER_TOKEN_PATH"
}

seed_node_record() {
  local uuid name ip version concurrency

  uuid="${NODE_ID}"
  name="${NODE_NAME:-local-node-0001}"
  ip="${NODE_IP:-127.0.0.1}"
  version="${NODE_VERSION:-dev}"
  concurrency="${NODE_CONCURRENCY:-1}"

  log "Seeding node record in ploy.nodes..."
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO nodes (id, name, ip_address, version, concurrency)
    VALUES ('${uuid}', '${name}', '${ip}', '${version}', ${concurrency})
    ON CONFLICT (id) DO NOTHING;"
}

wire_local_cli_descriptor() {
  local server_url="http://localhost:${PLOY_SERVER_PORT}"
  log "Wiring local CLI descriptor..."
  mkdir -p "$PLOY_CONFIG_HOME/clusters"
  cat > "$PLOY_CONFIG_HOME/clusters/local.json" <<JSON
{
  "cluster_id": "${CLUSTER_ID}",
  "address": "${server_url}",
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
  if [[ -x "./dist/ploy" ]]; then
    PLOY_CONFIG_HOME="$PLOY_CONFIG_HOME" ./dist/ploy cluster token list || true
  fi
}

main() {
  local admin_pg_dsn
  local target_server=0
  local target_node=0
  local -a runtime_build_services=(garage-init)
  local -a compose_services=(garage garage-init gradle-build-cache)

  parse_args "$@"

  log "Checking prerequisites..."
  need podman
  need podman-compose
  need "$PYTHON_BIN"
  need openssl
  need make
  need curl
  need psql
  need pg_isready
  resolve_container_socket_path
  log "Using container socket path: ${PLOY_CONTAINER_SOCKET_PATH}"

  if [[ -z "$PLOY_DB_DSN" ]]; then
    echo "error: PLOY_DB_DSN is required (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)" >&2
    exit 1
  fi

  PLOY_DB_DSN="$(normalize_container_pg_dsn)"
  validate_container_pg_dsn

  admin_pg_dsn="$(derive_admin_pg_dsn)"

  wait_for_postgres "$admin_pg_dsn"
  if [[ $DROP_DB -eq 1 ]]; then
    drop_and_recreate_ploy_db "$admin_pg_dsn"
  else
    ensure_ploy_db_exists "$admin_pg_dsn"
  fi

  log "Building CLI/binaries (make build)..."
  make build

  if [[ ! -f "dist/ployd-linux" ]]; then
    echo "error: dist/ployd-linux not found after build" >&2
    exit 1
  fi
  if [[ ! -f "dist/ployd-node-linux" ]]; then
    echo "error: dist/ployd-node-linux not found after build" >&2
    exit 1
  fi

  if [[ ! -f "$AUTH_SECRET_PATH" ]]; then
    log "Generating auth secret at $AUTH_SECRET_PATH..."
    mkdir -p "$(dirname "$AUTH_SECRET_PATH")"
    openssl rand -hex 32 > "$AUTH_SECRET_PATH"
  fi

  export PLOY_AUTH_SECRET
  PLOY_AUTH_SECRET="$(cat "$AUTH_SECRET_PATH")"
  export CLUSTER_ID
  export PLOY_DB_DSN
  export PLOY_CONTAINER_SOCKET_PATH
  export PLOY_SERVER_PORT

  if [[ $REFRESH_PLOYD -eq 0 && $REFRESH_NODES -eq 0 ]]; then
    target_server=1
    target_node=1
  else
    if [[ $REFRESH_PLOYD -eq 1 ]]; then
      target_server=1
    fi
    if [[ $REFRESH_NODES -eq 1 ]]; then
      target_node=1
    fi
  fi

  # Node service depends on server health in compose, so include server when refreshing node.
  if [[ $target_node -eq 1 ]]; then
    target_server=1
  fi

  if [[ $target_server -eq 1 ]]; then
    runtime_build_services+=(server)
    compose_services+=(server)
  fi
  if [[ $target_node -eq 1 ]]; then
    runtime_build_services+=(node)
    compose_services+=(node)
  fi

  log "Generating admin and worker JWT tokens..."
  # shellcheck disable=SC2046
  eval "$(generate_tokens)"

  if [[ $target_node -eq 1 ]]; then
    provision_worker_token_into_node
  fi

  build_runtime_images "${runtime_build_services[@]}"
  remove_stale_service_containers "${compose_services[@]}"

  log "Starting local podman stack with: $COMPOSE_CMD up -d --no-build ${compose_services[*]}"
  $COMPOSE_CMD up -d --no-build "${compose_services[@]}"

  wait_for_garage_bootstrap
  wait_for_server_health

  seed_tokens

  if [[ $target_node -eq 1 ]]; then
    seed_node_record
  fi

  wire_local_cli_descriptor

  log "Local Ploy cluster is up."
  log "Admin JWT (save securely):"
  echo "$ADMIN_TOKEN"
}

main "$@"
