#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNTIME_DIR="$SCRIPT_DIR"
cd "$RUNTIME_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f ${RUNTIME_DIR}/docker-compose.yml}"
PLOY_CONTAINER_REGISTRY="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-docker}"
CLUSTER_ID="${CLUSTER_ID:-local}"
NODE_ID="${NODE_ID:-local1}"
PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy}"
AUTH_JSON_PATH=""
PLOY_DB_DSN="${PLOY_DB_DSN:-}"
PLOY_DB_DSN_HOST=""
PLOY_DB_DSN_CONTAINER=""
PLOY_DEPLOY_CA_BUNDLE="${PLOY_DEPLOY_CA_BUNDLE:-}"
PLOY_CONTAINER_SOCKET_PATH="${PLOY_CONTAINER_SOCKET_PATH:-/var/run/docker.sock}"
PLOY_SERVER_PORT="${PLOY_SERVER_PORT:-8080}"
PLOY_VERSION="${PLOY_VERSION:-$(ploy --version | head -n 1)}"
WORKER_TOKEN_PATH=""
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
Usage: ploy cluster deploy [--drop-db] [--ployd] [--nodes] [--no-pull] [--cluster <id>] [cluster]

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
  PLOY_DEPLOY_CA_BUNDLE           Optional path to PEM CA bundle used for docker daemon trust and runtime container trust
  PLOY_VERSION            Runtime version tag (default from `ploy --version`, example v0.1.0)
  PLOY_RUNTIME_SERVER_IMAGE   Runtime server image (default ghcr.io/iw2rmb/ploy/server:${PLOY_VERSION})
  PLOY_RUNTIME_NODE_IMAGE     Runtime node image (default ghcr.io/iw2rmb/ploy/node:${PLOY_VERSION})
  PLOY_RUNTIME_PULL_IMAGES Set to 0/false to skip pull before up (default: 1)
  PLOY_CONFIG_HOME        Config root (default: $HOME/.config/ploy)
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

init_runtime_image_defaults() {
  : "${PLOY_RUNTIME_SERVER_IMAGE:=ghcr.io/iw2rmb/ploy/server:${PLOY_VERSION}}"
  : "${PLOY_RUNTIME_NODE_IMAGE:=ghcr.io/iw2rmb/ploy/node:${PLOY_VERSION}}"
  export PLOY_VERSION
  export PLOY_RUNTIME_SERVER_IMAGE
  export PLOY_RUNTIME_NODE_IMAGE
}

derive_admin_pg_dsn() {
  local dsn="$1"
  printf '%s\n' "$dsn" | sed -E 's#(/)ploy([/?#]|$)#\1postgres\2#'
}

normalize_container_pg_dsn() {
  local dsn="$1"
  printf '%s\n' "$dsn" | sed -E \
    -e 's#(://|@)localhost([:/?#]|$)#\1host.docker.internal\2#g' \
    -e 's#(://|@)127\.0\.0\.1([:/?#]|$)#\1host.docker.internal\2#g' \
    -e 's#(://|@)\[::1\]([:/?#]|$)#\1host.docker.internal\2#g'
}

normalize_host_pg_dsn() {
  local dsn="$1"
  printf '%s\n' "$dsn" | sed -E \
    -e 's#(://|@)host\.docker\.internal([:/?#]|$)#\1localhost\2#g' \
    -e 's#(://|@)docker\.for\.mac\.host\.internal([:/?#]|$)#\1localhost\2#g' \
    -e 's#(://|@)gateway\.docker\.internal([:/?#]|$)#\1localhost\2#g'
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

  log "Installing PLOY_DEPLOY_CA_BUNDLE into colima docker registry trust..."
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

  log "Installing PLOY_DEPLOY_CA_BUNDLE into local docker registry trust..."
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

  if [[ -z "$PLOY_DEPLOY_CA_BUNDLE" ]]; then
    return 0
  fi

  ca_path="$(resolve_ca_bundle_path "$PLOY_DEPLOY_CA_BUNDLE")"
  PLOY_DEPLOY_CA_BUNDLE="$ca_path"
  context="$(docker context show 2>/dev/null || true)"
  os_name="$(uname -s)"
  engine_name="$(docker info --format '{{.Name}}' 2>/dev/null || true)"

  if [[ "$context" == "colima" || "$engine_name" == "colima" ]]; then
    install_registry_ca_colima "$PLOY_DEPLOY_CA_BUNDLE"
    wait_for_docker_engine_ready
    return
  fi
  if [[ "$os_name" == "Linux" ]]; then
    install_registry_ca_linux "$PLOY_DEPLOY_CA_BUNDLE"
    wait_for_docker_engine_ready
    return
  fi

  echo "error: PLOY_DEPLOY_CA_BUNDLE auto-install is not supported for docker context '${context}' (engine='${engine_name}') on ${os_name}" >&2
  echo "error: configure docker daemon trust manually, then rerun ploy cluster deploy" >&2
  exit 1
}

resolve_runtime_ca_bundle() {
  if [[ -z "${PLOY_DEPLOY_CA_BUNDLE:-}" ]]; then
    PLOY_DEPLOY_CA_BUNDLE="/dev/null"
    return 0
  fi

  PLOY_DEPLOY_CA_BUNDLE="$(resolve_ca_bundle_path "$PLOY_DEPLOY_CA_BUNDLE")"
}

resolve_ca_bundle_path() {
  local ca_path="$1"

  if [[ "$ca_path" != /* ]]; then
    ca_path="$RUNTIME_DIR/$ca_path"
  fi
  if [[ ! -f "$ca_path" ]]; then
    echo "error: PLOY_DEPLOY_CA_BUNDLE file not found: $ca_path" >&2
    exit 1
  fi
  if [[ ! -s "$ca_path" ]]; then
    echo "error: PLOY_DEPLOY_CA_BUNDLE file is empty: $ca_path" >&2
    exit 1
  fi

  printf '%s\n' "$ca_path"
}

generate_tokens() {
  local secret
  local admin_token admin_id admin_hash
  local worker_token worker_id worker_hash

  b64url() {
    openssl base64 -A | tr '+/' '-_' | tr -d '='
  }

  sha256_hex() {
    openssl dgst -sha256 | awk '{print $NF}'
  }

  gen_token() {
    local role="$1"
    local now exp jti header payload header_b64 payload_b64 unsigned signature token token_hash

    now="$(date +%s)"
    exp="$((now + 365 * 24 * 60 * 60))"
    jti="$(openssl rand 16 | b64url)"
    header='{"alg":"HS256","typ":"JWT"}'
    payload="$(jq -cn \
      --arg cluster_id "$CLUSTER_ID" \
      --arg role "$role" \
      --arg jti "$jti" \
      --argjson iat "$now" \
      --argjson exp "$exp" \
      '{cluster_id:$cluster_id,role:$role,token_type:"api",iat:$iat,exp:$exp,jti:$jti}')"

    header_b64="$(printf '%s' "$header" | b64url)"
    payload_b64="$(printf '%s' "$payload" | b64url)"
    unsigned="${header_b64}.${payload_b64}"
    signature="$(printf '%s' "$unsigned" | openssl dgst -sha256 -hmac "$PLOY_AUTH_SECRET" -binary | b64url)"
    token="${unsigned}.${signature}"
    token_hash="$(printf '%s' "$token" | sha256_hex)"

    printf '%s|%s|%s\n' "$token" "$jti" "$token_hash"
  }

  IFS='|' read -r admin_token admin_id admin_hash < <(gen_token "cli-admin")
  IFS='|' read -r worker_token worker_id worker_hash < <(gen_token "worker")

  printf 'ADMIN_TOKEN=%s\n' "$admin_token"
  printf 'ADMIN_TOKEN_ID=%s\n' "$admin_id"
  printf 'ADMIN_TOKEN_HASH=%s\n' "$admin_hash"
  printf 'WORKER_TOKEN=%s\n' "$worker_token"
  printf 'WORKER_TOKEN_ID=%s\n' "$worker_id"
  printf 'WORKER_TOKEN_HASH=%s\n' "$worker_hash"
}

read_auth_json_field() {
  local field="$1"
  [[ -f "$AUTH_JSON_PATH" ]] || return 1
  jq -er --arg field "$field" '.[$field] // "" | tostring' "$AUTH_JSON_PATH"
}

write_auth_json() {
  local auth_dir server_url generated_at
  auth_dir="$(dirname "$AUTH_JSON_PATH")"
  server_url="http://127.0.0.1:${PLOY_SERVER_PORT}"
  generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  mkdir -p "$auth_dir"

  cat <<EOF > $AUTH_JSON_PATH
{
    "cluster_id": "$CLUSTER_ID",
    "node_id": "$NODE_ID",
    "address": "$server_url",
    "token": "$ADMIN_TOKEN",
    "auth_secret": "$PLOY_AUTH_SECRET",
    "admin_token": "$ADMIN_TOKEN",
    "admin_token_id": "$ADMIN_TOKEN_ID",
    "admin_token_hash": "$ADMIN_TOKEN_HASH",
    "worker_token": "$WORKER_TOKEN",
    "worker_token_id": "$WORKER_TOKEN_ID",
    "worker_token_hash": "$WORKER_TOKEN_HASH",
    "generated_at": "$generated_at"
}
EOF

  chmod 600 "$AUTH_JSON_PATH"
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

configure_runtime_globals_and_persist_auth() {
  log "Writing cluster auth state to ${AUTH_JSON_PATH}..."
  write_auth_json
  set_default_auth_symlink

  log "Configuring Gradle Build Cache globals..."
  if ! ploy config env set --key PLOY_GRADLE_BUILD_CACHE_URL --value "http://gradle-build-cache:5071/cache/" --on gates >/dev/null; then
    echo "error: failed to set PLOY_GRADLE_BUILD_CACHE_URL via ploy config env set --on gates" >&2
    exit 1
  fi
  if ! ploy config env set --key PLOY_GRADLE_BUILD_CACHE_PUSH --value "true" --on gates >/dev/null; then
    echo "error: failed to set PLOY_GRADLE_BUILD_CACHE_PUSH via ploy config env set --on gates" >&2
    exit 1
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
  init_runtime_image_defaults

  log "Checking prerequisites..."
  need docker
  need jq
  need openssl
  need psql
  need pg_isready
  need ploy

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

  PLOY_DB_DSN_HOST="$(normalize_host_pg_dsn "$PLOY_DB_DSN")"
  PLOY_DB_DSN_CONTAINER="$(normalize_container_pg_dsn "$PLOY_DB_DSN_HOST")"
  PLOY_DB_DSN="$PLOY_DB_DSN_CONTAINER"

  admin_pg_dsn="$(derive_admin_pg_dsn "$PLOY_DB_DSN_HOST")"
  wait_for_postgres "$admin_pg_dsn"
  if [[ $DROP_DB -eq 1 ]]; then
    drop_and_recreate_ploy_db "$admin_pg_dsn"
  else
    ensure_ploy_db_exists "$admin_pg_dsn"
  fi

  AUTH_JSON_PATH="$PLOY_CONFIG_HOME/$CLUSTER_ID/auth.json"
  WORKER_TOKEN_PATH="$PLOY_CONFIG_HOME/$CLUSTER_ID/bearer-token"

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
  export PLOY_CONTAINER_SOCKET_PATH
  export PLOY_SERVER_PORT
  export PLOY_DEPLOY_CA_BUNDLE
  export WORKER_TOKEN_PATH
  export PLOY_CONTAINER_REGISTRY

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
