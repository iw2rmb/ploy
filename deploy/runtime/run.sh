#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/docker-compose.yml" ]]; then
  ROOT_DIR="$SCRIPT_DIR"
  RUNTIME_DIR="$SCRIPT_DIR"
else
  ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
  RUNTIME_DIR="$ROOT_DIR/deploy/runtime"
fi
cd "$ROOT_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f ${RUNTIME_DIR}/docker-compose.yml}"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-docker}"
CLUSTER_ID="${CLUSTER_ID:-local}"
NODE_ID="${NODE_ID:-local1}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy}"
AUTH_JSON_PATH="${AUTH_JSON_PATH:-}"
PLOY_DB_DSN="${PLOY_DB_DSN:-}"
PLOY_DB_DSN_HOST=""
PLOY_DB_DSN_CONTAINER=""
PLOY_CA_CERTS="${PLOY_CA_CERTS:-}"
PLOY_CONTAINER_SOCKET_PATH="${PLOY_CONTAINER_SOCKET_PATH:-/var/run/docker.sock}"
PLOY_SERVER_PORT="${PLOY_SERVER_PORT:-8080}"
PLOY_VERSION="${PLOY_VERSION:-}"
WORKER_TOKEN_PATH="${WORKER_TOKEN_PATH:-}"
PULL_IMAGES="${PLOY_RUNTIME_PULL_IMAGES:-1}"

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
Usage: ./deploy/runtime/run.sh [--drop-db] [--ployd] [--nodes] [--no-pull] [--cluster <id>] [cluster]

Runtime deploy using pre-built GHCR images.

Options:
  --drop-db  Drop and recreate the ploy database before deploy
  --ployd    Refresh/deploy server only
  --nodes    Refresh/deploy node (includes required server dependency)
  --no-pull  Skip docker compose pull before up
  --cluster  Cluster id (default: local). You can also pass it as a positional arg.

Environment:
  PLOY_DB_DSN             PostgreSQL DSN used by host setup and server container
  PLOY_OBJECTSTORE_ENDPOINT   S3-compatible endpoint URL used by server object store config
  PLOY_OBJECTSTORE_ACCESS_KEY S3 access key used by server object store config
  PLOY_OBJECTSTORE_SECRET_KEY S3 secret key used by server object store config
  PLOY_CA_CERTS           Optional path to PEM CA bundle used for docker daemon trust and runtime container trust
  PLOY_VERSION            Runtime version tag (default from ./VERSION, example v0.1.0)
  PLOY_RUNTIME_SERVER_IMAGE   Runtime server image (default ghcr.io/iw2rmb/ploy/server:${PLOY_VERSION})
  PLOY_RUNTIME_NODE_IMAGE     Runtime node image (default ghcr.io/iw2rmb/ploy/node:${PLOY_VERSION})
  PLOY_RUNTIME_PULL_IMAGES Set to 0/false to skip pull before up (default: 1)
  PLOY_CONFIG_HOME        Config root (default: $HOME/.config/ploy)
  AUTH_JSON_PATH          Optional explicit auth state path; default: <PLOY_CONFIG_HOME>/<cluster>/auth.json
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
      --no-pull)
        PULL_IMAGES=0
        ;;
      --cluster)
        shift
        if [[ $# -eq 0 || -z "$1" ]]; then
          echo "error: --cluster requires a value" >&2
          exit 1
        fi
        CLUSTER_ID="$1"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      -*)
        echo "error: unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
      *)
        CLUSTER_ID="$1"
        ;;
    esac
    shift
  done
}

init_cluster_paths() {
  local cfg_root
  cfg_root="${PLOY_CONFIG_HOME}"
  if [[ "$cfg_root" != /* ]]; then
    cfg_root="$ROOT_DIR/$cfg_root"
  fi

  if [[ -z "$AUTH_JSON_PATH" ]]; then
    AUTH_JSON_PATH="$cfg_root/$CLUSTER_ID/auth.json"
  elif [[ "$AUTH_JSON_PATH" != /* ]]; then
    AUTH_JSON_PATH="$ROOT_DIR/$AUTH_JSON_PATH"
  fi

  if [[ -z "$WORKER_TOKEN_PATH" ]]; then
    WORKER_TOKEN_PATH="$cfg_root/$CLUSTER_ID/bearer-token"
  elif [[ "$WORKER_TOKEN_PATH" != /* ]]; then
    WORKER_TOKEN_PATH="$ROOT_DIR/$WORKER_TOKEN_PATH"
  fi

  PLOY_CONFIG_HOME="$cfg_root"
}

resolve_ploy_version() {
  if [[ -z "$PLOY_VERSION" && -f "$ROOT_DIR/VERSION" ]]; then
    PLOY_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/VERSION")"
  fi
  if [[ -z "$PLOY_VERSION" ]]; then
    echo "error: PLOY_VERSION is required (set env or create $ROOT_DIR/VERSION)" >&2
    exit 1
  fi
  if [[ ! "$PLOY_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
    echo "error: PLOY_VERSION must be semver (vX.Y.Z or vX.Y.Z-prerelease), got '$PLOY_VERSION'" >&2
    exit 1
  fi
}

init_runtime_image_defaults() {
  : "${PLOY_RUNTIME_SERVER_IMAGE:=ghcr.io/iw2rmb/ploy/server:${PLOY_VERSION}}"
  : "${PLOY_RUNTIME_NODE_IMAGE:=ghcr.io/iw2rmb/ploy/node:${PLOY_VERSION}}"
  export PLOY_VERSION
  export PLOY_RUNTIME_SERVER_IMAGE
  export PLOY_RUNTIME_NODE_IMAGE
}

derive_admin_pg_dsn() {
  "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import urlsplit, urlunsplit

dsn = os.environ["PLOY_DB_DSN"].strip()
if not dsn:
    raise SystemExit("error: PLOY_DB_DSN is required")
if "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN")
u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")
if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")
print(urlunsplit((u.scheme, u.netloc, "/postgres", u.query, u.fragment)))
PY
}

normalize_container_pg_dsn() {
  PLOY_DB_DSN="$PLOY_DB_DSN" USER="${USER:-}" "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import quote, urlsplit, urlunsplit

dsn = os.environ["PLOY_DB_DSN"].strip()
if not dsn or "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN")
u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")
if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")

username = u.username or os.environ.get("USER", "").strip()
if not username:
    raise SystemExit("error: unable to infer postgres username; include username in PLOY_DB_DSN or set USER")

password = u.password
host = (u.hostname or "").lower()
if host in ("localhost", "127.0.0.1", "::1"):
    host = "host.docker.internal"
if ":" in host and not host.startswith("["):
    host = f"[{host}]"

userinfo = quote(username, safe="")
if password is not None:
    userinfo += ":" + quote(password, safe="")

port = f":{u.port}" if u.port else ""
netloc = f"{userinfo}@{host}{port}"
print(urlunsplit((u.scheme, netloc, u.path, u.query, u.fragment)))
PY
}

normalize_host_pg_dsn() {
  PLOY_DB_DSN="$PLOY_DB_DSN" USER="${USER:-}" "$PYTHON_BIN" <<'PY'
import os
from urllib.parse import quote, urlsplit, urlunsplit

dsn = os.environ["PLOY_DB_DSN"].strip()
if not dsn or "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN")
u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")
if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")

username = u.username or os.environ.get("USER", "").strip()
if not username:
    raise SystemExit("error: unable to infer postgres username; include username in PLOY_DB_DSN or set USER")

password = u.password
host = u.hostname or ""
if host.lower() in ("host.docker.internal", "docker.for.mac.host.internal", "gateway.docker.internal"):
    host = "localhost"
if ":" in host and not host.startswith("["):
    host = f"[{host}]"

userinfo = quote(username, safe="")
if password is not None:
    userinfo += ":" + quote(password, safe="")

port = f":{u.port}" if u.port else ""
netloc = f"{userinfo}@{host}{port}"
print(urlunsplit((u.scheme, netloc, u.path, u.query, u.fragment)))
PY
}

wait_for_postgres() {
  local admin_dsn="$1"
  local status=""
  log "Waiting for local PostgreSQL to be ready..."
  for i in {1..60}; do
    if status="$(PGCONNECT_TIMEOUT=2 pg_isready -d "$admin_dsn" 2>&1)"; then
      return 0
    fi
    if (( i == 1 || i % 10 == 0 )); then
      log "PostgreSQL not ready yet (${i}/60): ${status}"
    fi
    sleep 1
  done
  echo "error: local PostgreSQL did not become ready in time (last status: ${status})" >&2
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

run_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  return 1
}

wait_for_docker_engine_ready() {
  for _ in {1..30}; do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "error: docker engine did not become ready in time after CA installation" >&2
  exit 1
}

install_registry_ca_colima() {
  local ca_path="$1"
  local -a registries=(docker.io registry-1.docker.io auth.docker.io index.docker.io ghcr.io)

  if ! command -v colima >/dev/null 2>&1; then
    echo "error: docker context is colima but colima CLI is not installed" >&2
    exit 1
  fi

  log "Installing PLOY_CA_CERTS into colima docker registry trust..."
  for reg in "${registries[@]}"; do
    colima ssh -- sudo mkdir -p "/etc/docker/certs.d/${reg}"
    cat "$ca_path" | colima ssh -- sudo tee "/etc/docker/certs.d/${reg}/ca.crt" >/dev/null
    colima ssh -- sudo chmod 0644 "/etc/docker/certs.d/${reg}/ca.crt"
  done
  colima ssh -- sudo mkdir -p /usr/local/share/ca-certificates/ploy
  cat "$ca_path" | colima ssh -- sudo tee /usr/local/share/ca-certificates/ploy/ploy-ca.crt >/dev/null
  colima ssh -- sudo update-ca-certificates >/dev/null
  colima ssh -- sudo systemctl restart docker
}

install_registry_ca_linux() {
  local ca_path="$1"
  local -a registries=(docker.io registry-1.docker.io auth.docker.io index.docker.io ghcr.io)

  log "Installing PLOY_CA_CERTS into local docker registry trust..."
  for reg in "${registries[@]}"; do
    if ! run_as_root mkdir -p "/etc/docker/certs.d/${reg}"; then
      echo "error: cannot create /etc/docker/certs.d/${reg}" >&2
      exit 1
    fi
    if ! run_as_root install -m 0644 "$ca_path" "/etc/docker/certs.d/${reg}/ca.crt"; then
      echo "error: cannot install CA bundle to /etc/docker/certs.d/${reg}/ca.crt" >&2
      exit 1
    fi
  done

  if ! run_as_root mkdir -p /usr/local/share/ca-certificates/ploy; then
    echo "error: cannot create /usr/local/share/ca-certificates/ploy" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /usr/local/share/ca-certificates/ploy/ploy-ca.crt; then
    echo "error: cannot install CA bundle to /usr/local/share/ca-certificates/ploy/ploy-ca.crt" >&2
    exit 1
  fi
  if command -v update-ca-certificates >/dev/null 2>&1; then
    run_as_root update-ca-certificates >/dev/null || {
      echo "error: failed to update system CA trust" >&2
      exit 1
    }
  fi

  if command -v systemctl >/dev/null 2>&1; then
    run_as_root systemctl restart docker || {
      echo "error: failed to restart docker via systemctl" >&2
      exit 1
    }
    return
  fi
  if command -v service >/dev/null 2>&1; then
    run_as_root service docker restart || {
      echo "error: failed to restart docker via service" >&2
      exit 1
    }
    return
  fi

  echo "error: installed CA bundle but could not restart docker daemon" >&2
  exit 1
}

configure_docker_registry_ca_if_needed() {
  local ca_path context os_name engine_name

  if [[ -z "$PLOY_CA_CERTS" ]]; then
    return 0
  fi

  ca_path="$PLOY_CA_CERTS"
  if [[ "$ca_path" != /* ]]; then
    ca_path="$ROOT_DIR/$ca_path"
  fi
  if [[ ! -f "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file not found: $ca_path" >&2
    exit 1
  fi
  if [[ ! -s "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file is empty: $ca_path" >&2
    exit 1
  fi

  PLOY_CA_CERTS="$ca_path"
  context="$(docker context show 2>/dev/null || true)"
  os_name="$(uname -s)"
  engine_name="$(docker info --format '{{.Name}}' 2>/dev/null || true)"

  if [[ "$context" == "colima" || "$engine_name" == "colima" ]]; then
    install_registry_ca_colima "$PLOY_CA_CERTS"
    wait_for_docker_engine_ready
    return
  fi
  if [[ "$os_name" == "Linux" ]]; then
    install_registry_ca_linux "$PLOY_CA_CERTS"
    wait_for_docker_engine_ready
    return
  fi

  echo "error: PLOY_CA_CERTS auto-install is not supported for docker context '${context}' (engine='${engine_name}') on ${os_name}" >&2
  echo "error: configure docker daemon trust manually, then rerun deploy/runtime/run.sh" >&2
  exit 1
}

resolve_runtime_ca_bundle() {
  local ca_path="${PLOY_CA_CERTS:-}"

  if [[ -z "$ca_path" ]]; then
    PLOY_CA_CERTS="/dev/null"
    return 0
  fi

  if [[ "$ca_path" != /* ]]; then
    ca_path="$ROOT_DIR/$ca_path"
  fi
  if [[ ! -f "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file not found: $ca_path" >&2
    exit 1
  fi
  if [[ ! -s "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file is empty: $ca_path" >&2
    exit 1
  fi

  PLOY_CA_CERTS="$ca_path"
}

generate_tokens() {
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

read_auth_json_field() {
  local field="$1"
  [[ -f "$AUTH_JSON_PATH" ]] || return 1
  AUTH_JSON_PATH="$AUTH_JSON_PATH" AUTH_FIELD="$field" "$PYTHON_BIN" <<'PY'
import json
import os

path = os.environ["AUTH_JSON_PATH"]
field = os.environ["AUTH_FIELD"]
try:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    raise SystemExit(1)
value = data.get(field, "")
if value is None:
    value = ""
print(str(value))
PY
}

write_auth_json() {
  local auth_dir server_url tmp_file
  auth_dir="$(dirname "$AUTH_JSON_PATH")"
  server_url="http://127.0.0.1:${PLOY_SERVER_PORT}"
  mkdir -p "$auth_dir"
  tmp_file="$(mktemp)"
  AUTH_JSON_PATH="$AUTH_JSON_PATH" \
  CLUSTER_ID="$CLUSTER_ID" \
  NODE_ID="$NODE_ID" \
  SERVER_URL="$server_url" \
  PLOY_AUTH_SECRET="$PLOY_AUTH_SECRET" \
  ADMIN_TOKEN="$ADMIN_TOKEN" \
  ADMIN_TOKEN_ID="$ADMIN_TOKEN_ID" \
  ADMIN_TOKEN_HASH="$ADMIN_TOKEN_HASH" \
  WORKER_TOKEN="$WORKER_TOKEN" \
  WORKER_TOKEN_ID="$WORKER_TOKEN_ID" \
  WORKER_TOKEN_HASH="$WORKER_TOKEN_HASH" \
  "$PYTHON_BIN" <<'PY' > "$tmp_file"
import json
import os
from datetime import datetime, timezone

payload = {
    "cluster_id": os.environ["CLUSTER_ID"],
    "node_id": os.environ["NODE_ID"],
    "address": os.environ["SERVER_URL"],
    "token": os.environ["ADMIN_TOKEN"],
    "auth_secret": os.environ["PLOY_AUTH_SECRET"],
    "admin_token": os.environ["ADMIN_TOKEN"],
    "admin_token_id": os.environ["ADMIN_TOKEN_ID"],
    "admin_token_hash": os.environ["ADMIN_TOKEN_HASH"],
    "worker_token": os.environ["WORKER_TOKEN"],
    "worker_token_id": os.environ["WORKER_TOKEN_ID"],
    "worker_token_hash": os.environ["WORKER_TOKEN_HASH"],
    "generated_at": datetime.now(timezone.utc).isoformat(),
}
print(json.dumps(payload, indent=2, sort_keys=True))
PY
  chmod 600 "$tmp_file"
  mv "$tmp_file" "$AUTH_JSON_PATH"
}

set_default_auth_symlink() {
  local default_link

  default_link="$PLOY_CONFIG_HOME/default"
  mkdir -p "$PLOY_CONFIG_HOME"

  if [[ -d "$default_link" && ! -L "$default_link" ]]; then
    echo "error: cannot set default cluster marker at ${default_link}: path is a directory" >&2
    exit 1
  fi

  rm -f "$default_link"
  ln -s "$AUTH_JSON_PATH" "$default_link"
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

wait_for_server_health() {
  local server_cid server_health server_state server_exit

  server_cid="$($COMPOSE_CMD ps -a -q server)"
  if [[ -z "$server_cid" ]]; then
    echo "error: could not resolve server container ID" >&2
    $COMPOSE_CMD ps || true
    exit 1
  fi

  log "Waiting for server container health..."
  for _ in {1..90}; do
    server_health="$($CONTAINER_ENGINE inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$server_cid" 2>/dev/null || true)"
    server_state="$($CONTAINER_ENGINE inspect -f '{{.State.Status}}' "$server_cid" 2>/dev/null || true)"
    server_exit="$($CONTAINER_ENGINE inspect -f '{{.State.ExitCode}}' "$server_cid" 2>/dev/null || true)"

    if [[ "$server_health" == "healthy" ]]; then
      return 0
    fi
    if [[ "$server_health" == "none" && "$server_state" == "running" ]]; then
      return 0
    fi
    if [[ "$server_state" == "exited" || "$server_state" == "dead" ]]; then
      echo "error: server container is ${server_state} (exit=${server_exit})" >&2
      $COMPOSE_CMD logs server || true
      exit 1
    fi

    sleep 1
  done

  echo "error: server container did not become healthy in time (state=${server_state}, health=${server_health})" >&2
  $COMPOSE_CMD logs server || true
  exit 1
}

seed_tokens() {
  log "Inserting admin token into api_tokens..."
  psql "$PLOY_DB_DSN_HOST" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES ('${ADMIN_TOKEN_HASH}', '${ADMIN_TOKEN_ID}', '${CLUSTER_ID}', 'cli-admin', 'Runtime deploy admin token', NOW(), NOW() + INTERVAL '365 days')
    ON CONFLICT (token_hash) DO NOTHING;"

  log "Inserting worker token into api_tokens..."
  psql "$PLOY_DB_DSN_HOST" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES ('${WORKER_TOKEN_HASH}', '${WORKER_TOKEN_ID}', '${CLUSTER_ID}', 'worker', 'Runtime deploy worker token for node ${NODE_ID}', NOW(), NOW() + INTERVAL '365 days')
    ON CONFLICT (token_hash) DO NOTHING;"
}

seed_node_record() {
  log "Seeding node record in ploy.nodes..."
  psql "$PLOY_DB_DSN_HOST" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO nodes (id, name, ip_address, version, concurrency)
    VALUES ('${NODE_ID}', 'runtime-node-0001', '127.0.0.1', 'runtime', 1)
    ON CONFLICT (id) DO NOTHING;"
}

set_global_env_via_server_api() {
  local key="$1"
  local value="$2"
  local target="${3:-gates}"
  local payload=""
  local response=""
  local http_code=""
  local response_body=""
  local legacy_scope=""

  global_env_payload() {
    local field_name="$1"
    local field_value="$2"
    GLOBAL_ENV_VALUE="$value" GLOBAL_ENV_FIELD_NAME="$field_name" GLOBAL_ENV_FIELD_VALUE="$field_value" "$PYTHON_BIN" <<'PY'
import json
import os
print(json.dumps({
  "value": os.environ["GLOBAL_ENV_VALUE"],
  os.environ["GLOBAL_ENV_FIELD_NAME"]: os.environ["GLOBAL_ENV_FIELD_VALUE"],
  "secret": True,
}))
PY
  }

  legacy_scope_from_target() {
    local target_name="$1"
    case "$target_name" in
      gates) echo "gate" ;;
      all) echo "all" ;;
      *)
        echo ""
        return 1
        ;;
    esac
  }

  put_global_env() {
    local req_payload="$1"
    $COMPOSE_CMD exec -T \
      -e PLOY_ADMIN_TOKEN="$ADMIN_TOKEN" \
      -e PLOY_ENV_KEY="$key" \
      -e PLOY_ENV_PAYLOAD="$req_payload" \
      server sh -lc '
        code="$(curl -sS -o /tmp/ploy_env_response.txt -w "%{http_code}" -X PUT "http://localhost:8080/v1/config/env/${PLOY_ENV_KEY}" \
          -H "Authorization: Bearer ${PLOY_ADMIN_TOKEN}" \
          -H "Content-Type: application/json" \
          --data "${PLOY_ENV_PAYLOAD}")"
        printf "%s\n" "$code"
        cat /tmp/ploy_env_response.txt
      '
  }

  payload="$(global_env_payload "target" "$target")"
  response="$(put_global_env "$payload")"
  http_code="${response%%$'\n'*}"
  response_body="${response#*$'\n'}"

  if [[ "$http_code" == "200" ]]; then
    return 0
  fi

  # Backward compatibility for older runtime server images that still use `scope`.
  if [[ "$http_code" == "400" && "$response_body" == *'unknown field "target"'* ]]; then
    legacy_scope="$(legacy_scope_from_target "$target" || true)"
    if [[ -z "$legacy_scope" ]]; then
      echo "error: failed to set ${key} through server API: target '${target}' is unsupported by legacy runtime API" >&2
      exit 1
    fi
    payload="$(global_env_payload "scope" "$legacy_scope")"
    response="$(put_global_env "$payload")"
    http_code="${response%%$'\n'*}"
    response_body="${response#*$'\n'}"
    if [[ "$http_code" == "200" ]]; then
      return 0
    fi
  fi

  echo "error: failed to set ${key} through server API (status ${http_code}): ${response_body}" >&2
  exit 1
}

runtime_ca_bundle_value() {
  if [[ -z "$PLOY_CA_CERTS" || "$PLOY_CA_CERTS" == "/dev/null" ]]; then
    return 0
  fi
  cat "$PLOY_CA_CERTS"
}

configure_runtime_globals_and_persist_auth() {
  local runtime_ca_bundle=""

  log "Writing cluster auth state to ${AUTH_JSON_PATH}..."
  write_auth_json
  set_default_auth_symlink

  log "Configuring Gradle Build Cache globals..."
  set_global_env_via_server_api PLOY_GRADLE_BUILD_CACHE_URL "http://gradle-build-cache:5071/cache/" gates
  set_global_env_via_server_api PLOY_GRADLE_BUILD_CACHE_PUSH "true" gates

  runtime_ca_bundle="$(runtime_ca_bundle_value || true)"
  if [[ -n "$runtime_ca_bundle" ]]; then
    log "Configuring global PLOY_CA_CERTS..."
    set_global_env_via_server_api PLOY_CA_CERTS "$runtime_ca_bundle" all
  fi
}

main() {
  local admin_pg_dsn
  local target_server=0
  local target_node=0
  local pull_images_flag=1
  local existing_auth_secret=""
  local -a compose_services=(gradle-build-cache)

  parse_args "$@"
  init_cluster_paths
  resolve_ploy_version
  init_runtime_image_defaults

  log "Checking prerequisites..."
  need docker
  need "$PYTHON_BIN"
  need openssl
  need psql
  need pg_isready

  case "$PULL_IMAGES" in
    0|false|FALSE|False|no|NO|No|off|OFF|Off)
      pull_images_flag=0
      ;;
  esac

  configure_docker_registry_ca_if_needed
  resolve_runtime_ca_bundle

  if [[ -z "$PLOY_DB_DSN" ]]; then
    echo "error: PLOY_DB_DSN is required (example: postgres://ploy:ploy@localhost:5432/ploy?sslmode=disable)" >&2
    exit 1
  fi
  if [[ -z "${PLOY_OBJECTSTORE_ENDPOINT:-}" || -z "${PLOY_OBJECTSTORE_ACCESS_KEY:-}" || -z "${PLOY_OBJECTSTORE_SECRET_KEY:-}" ]]; then
    echo "error: PLOY_OBJECTSTORE_ENDPOINT, PLOY_OBJECTSTORE_ACCESS_KEY, and PLOY_OBJECTSTORE_SECRET_KEY are required" >&2
    exit 1
  fi

  PLOY_DB_DSN_HOST="$(normalize_host_pg_dsn)"
  PLOY_DB_DSN="$PLOY_DB_DSN_HOST"
  PLOY_DB_DSN_CONTAINER="$(normalize_container_pg_dsn)"
  PLOY_DB_DSN="$PLOY_DB_DSN_CONTAINER"

  admin_pg_dsn="$(PLOY_DB_DSN="$PLOY_DB_DSN_HOST" derive_admin_pg_dsn)"
  wait_for_postgres "$admin_pg_dsn"
  if [[ $DROP_DB -eq 1 ]]; then
    drop_and_recreate_ploy_db "$admin_pg_dsn"
  else
    ensure_ploy_db_exists "$admin_pg_dsn"
  fi

  if existing_auth_secret="$(read_auth_json_field auth_secret 2>/dev/null)"; then
    PLOY_AUTH_SECRET="$existing_auth_secret"
    log "Reusing auth secret from ${AUTH_JSON_PATH}."
  else
    log "Generating auth secret for cluster '${CLUSTER_ID}'..."
    PLOY_AUTH_SECRET="$(openssl rand -hex 32)"
  fi

  export PLOY_AUTH_SECRET
  export CLUSTER_ID
  export PLOY_DB_DSN
  PLOY_DB_DSN="$PLOY_DB_DSN_CONTAINER"
  export PLOY_CONTAINER_SOCKET_PATH
  export PLOY_SERVER_PORT
  export PLOY_CA_CERTS
  export WORKER_TOKEN_PATH
  export PLOY_CONTAINER_REGISTRY
  if [[ -z "${PLOY_CONTAINER_REGISTRY:-}" ]]; then
    echo "error: PLOY_CONTAINER_REGISTRY is required (example: ghcr.io/iw2rmb/ploy)" >&2
    exit 1
  fi

  if [[ $REFRESH_PLOYD -eq 0 && $REFRESH_NODES -eq 0 ]]; then
    target_server=1
    target_node=1
  else
    [[ $REFRESH_PLOYD -eq 1 ]] && target_server=1
    [[ $REFRESH_NODES -eq 1 ]] && target_node=1
  fi

  if [[ $target_node -eq 1 ]]; then
    target_server=1
  fi
  if [[ $target_server -eq 1 ]]; then
    compose_services+=(server)
  fi
  if [[ $target_node -eq 1 ]]; then
    compose_services+=(node)
  fi

  log "Generating admin and worker JWT tokens..."
  # shellcheck disable=SC2046
  eval "$(generate_tokens)"

  if [[ $target_node -eq 1 ]]; then
    provision_worker_token_into_node
  fi

  if (( pull_images_flag )); then
    log "Pulling runtime images before start: ${compose_services[*]}"
    $COMPOSE_CMD pull "${compose_services[@]}"
  else
    log "Skipping image pull (PLOY_RUNTIME_PULL_IMAGES=${PLOY_RUNTIME_PULL_IMAGES:-$PULL_IMAGES})"
  fi

  log "Starting runtime docker stack with: $COMPOSE_CMD up -d ${compose_services[*]}"
  $COMPOSE_CMD up -d "${compose_services[@]}"

  wait_for_server_health

  seed_tokens
  if [[ $target_node -eq 1 ]]; then
    seed_node_record
  fi
  configure_runtime_globals_and_persist_auth

  log "Runtime Ploy cluster is up."
  log "Cluster auth state: ${AUTH_JSON_PATH}"
  log "Admin JWT (save securely): ${ADMIN_TOKEN}"
}

main "$@"
