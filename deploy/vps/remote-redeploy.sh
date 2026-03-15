#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-ploy-vps}"
COMPOSE_CMD=(docker compose --project-name "$COMPOSE_PROJECT_NAME" --env-file "$ROOT_DIR/stack.env" -f "$ROOT_DIR/docker-compose.yml")
PYTHON_BIN="${PYTHON_BIN:-python3}"
DROP_DB=0

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
Usage: ./remote-redeploy.sh [--drop-db]

Options:
  --drop-db  Drop and recreate the ploy database before deploy
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --drop-db)
        DROP_DB=1
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

wait_for_postgres() {
  local admin_dsn="$1"
  local last_status=""
  log "Waiting for PostgreSQL..."
  for i in {1..60}; do
    if last_status="$(PGCONNECT_TIMEOUT=2 pg_isready -d "$admin_dsn" 2>&1)"; then
      return 0
    fi
    if (( i == 1 || i % 10 == 0 )); then
      log "PostgreSQL not ready yet (${i}/60): ${last_status}"
    fi
    sleep 1
  done
  echo "error: PostgreSQL did not become ready in time (last status: ${last_status})" >&2
  exit 1
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

drop_and_recreate_ploy_db() {
  local admin_dsn="$1"

  log "Dropping and recreating database 'ploy'..."
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'ploy' AND pid <> pg_backend_pid();" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "DROP DATABASE IF EXISTS ploy;" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "CREATE DATABASE ploy;" >/dev/null
}

ensure_host_dirs() {
  log "Ensuring host workspace directories..."
  mkdir -p "$PLOY_NODE1_WORKDIR" "$PLOY_NODE2_WORKDIR"
}

load_images() {
  log "Loading bundled Docker images..."
  docker load -i "$ROOT_DIR/docker-images.tar" >/dev/null
}

wait_for_garage_bootstrap() {
  local garage_cid garage_init_cid garage_health init_state init_exit

  garage_cid="$("${COMPOSE_CMD[@]}" ps -a -q garage)"
  garage_init_cid="$("${COMPOSE_CMD[@]}" ps -a -q garage-init)"
  if [[ -z "$garage_cid" || -z "$garage_init_cid" ]]; then
    echo "error: could not resolve garage container IDs" >&2
    "${COMPOSE_CMD[@]}" ps || true
    exit 1
  fi

  log "Waiting for Garage health and bootstrap completion..."
  for _ in {1..90}; do
    garage_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$garage_cid" 2>/dev/null || true)"
    init_state="$(docker inspect -f '{{.State.Status}}' "$garage_init_cid" 2>/dev/null || true)"
    init_exit="$(docker inspect -f '{{.State.ExitCode}}' "$garage_init_cid" 2>/dev/null || true)"

    if [[ "$garage_health" == "healthy" && "$init_state" == "exited" && "$init_exit" == "0" ]]; then
      return 0
    fi

    if [[ "$init_state" == "exited" && "$init_exit" != "0" ]]; then
      echo "error: garage-init failed with exit code ${init_exit}" >&2
      "${COMPOSE_CMD[@]}" ps || true
      "${COMPOSE_CMD[@]}" logs garage garage-init || true
      exit 1
    fi

    sleep 1
  done

  echo "error: Garage bootstrap did not complete in time" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs garage garage-init || true
  exit 1
}

wait_for_registry_health() {
  log "Waiting for local registry readiness on http://127.0.0.1:${PLOY_REGISTRY_PORT}/v2/ ..."
  for _ in {1..90}; do
    if curl --noproxy '*' -fsS "http://127.0.0.1:${PLOY_REGISTRY_PORT}/v2/" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "error: registry did not become ready in time" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs registry || true
  exit 1
}

seed_registry_images() {
  local ref
  log "Pushing bundled workflow images into ${PLOY_CONTAINER_REGISTRY} ..."
  while IFS= read -r ref; do
    [[ -z "$ref" ]] && continue
    docker push "$ref" >/dev/null
  done < "$ROOT_DIR/workflow-images.txt"
}

wait_for_server_health() {
  local server_cid server_health server_state server_exit

  server_cid="$("${COMPOSE_CMD[@]}" ps -a -q server)"
  if [[ -z "$server_cid" ]]; then
    echo "error: could not resolve server container ID" >&2
    "${COMPOSE_CMD[@]}" ps || true
    exit 1
  fi

  log "Waiting for server container health..."
  for _ in {1..90}; do
    server_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$server_cid" 2>/dev/null || true)"
    server_state="$(docker inspect -f '{{.State.Status}}' "$server_cid" 2>/dev/null || true)"
    server_exit="$(docker inspect -f '{{.State.ExitCode}}' "$server_cid" 2>/dev/null || true)"

    if [[ "$server_health" == "healthy" ]]; then
      return 0
    fi
    if [[ "$server_health" == "none" && "$server_state" == "running" ]]; then
      return 0
    fi
    if [[ "$server_state" == "exited" || "$server_state" == "dead" ]]; then
      echo "error: server container is ${server_state} (exit=${server_exit})" >&2
      "${COMPOSE_CMD[@]}" ps || true
      "${COMPOSE_CMD[@]}" logs server || true
      exit 1
    fi
    sleep 1
  done

  echo "error: server container did not become healthy in time (state=${server_state}, health=${server_health})" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs server || true
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
      '${PLOY_CLUSTER_ID}',
      'cli-admin',
      'Offline VPS admin token',
      NOW(),
      NOW() + INTERVAL '365 days'
    )
    ON CONFLICT (token_hash) DO NOTHING;"

  log "Inserting worker tokens into api_tokens..."
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES
      (
        '${WORKER1_TOKEN_HASH}',
        '${WORKER1_TOKEN_ID}',
        '${PLOY_CLUSTER_ID}',
        'worker',
        'Offline VPS worker token for ${PLOY_NODE1_ID}',
        NOW(),
        NOW() + INTERVAL '365 days'
      ),
      (
        '${WORKER2_TOKEN_HASH}',
        '${WORKER2_TOKEN_ID}',
        '${PLOY_CLUSTER_ID}',
        'worker',
        'Offline VPS worker token for ${PLOY_NODE2_ID}',
        NOW(),
        NOW() + INTERVAL '365 days'
      )
    ON CONFLICT (token_hash) DO NOTHING;"
}

seed_node_records() {
  log "Seeding node records in ploy.nodes..."
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO nodes (id, name, ip_address, version, concurrency)
    VALUES
      ('${PLOY_NODE1_ID}', '${PLOY_NODE1_NAME}', '${PLOY_NODE1_IP}', 'dev', ${PLOY_NODE1_CONCURRENCY}),
      ('${PLOY_NODE2_ID}', '${PLOY_NODE2_NAME}', '${PLOY_NODE2_IP}', 'dev', ${PLOY_NODE2_CONCURRENCY})
    ON CONFLICT (id) DO NOTHING;"
}

set_global_env() {
  local key="$1"
  local value="$2"
  local scope="$3"
  local payload

  payload="$(GLOBAL_ENV_VALUE="$value" GLOBAL_ENV_SCOPE="$scope" "$PYTHON_BIN" <<'PY'
import json
import os

print(json.dumps({
    "value": os.environ["GLOBAL_ENV_VALUE"],
    "scope": os.environ["GLOBAL_ENV_SCOPE"],
    "secret": True,
}))
PY
)"

  curl -fsS -X PUT "http://127.0.0.1:${PLOY_SERVER_PORT}/v1/config/env/${key}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    --data "$payload" >/dev/null
}

configure_global_env() {
  log "Configuring remote Gradle Build Cache env..."
  set_global_env PLOY_GRADLE_BUILD_CACHE_URL "http://gradle-build-cache:5071/cache/" all
  set_global_env PLOY_GRADLE_BUILD_CACHE_PUSH "true" all
}

main() {
  local admin_pg_dsn

  parse_args "$@"

  need docker
  need psql
  need pg_isready
  need curl
  need "$PYTHON_BIN"

  # shellcheck disable=SC1091
  source "$ROOT_DIR/stack.env"
  # shellcheck disable=SC1091
  source "$ROOT_DIR/tokens.env"

  admin_pg_dsn="$(derive_admin_pg_dsn)"
  wait_for_postgres "$admin_pg_dsn"
  if (( DROP_DB )); then
    drop_and_recreate_ploy_db "$admin_pg_dsn"
  else
    ensure_ploy_db_exists "$admin_pg_dsn"
  fi

  ensure_host_dirs
  load_images

  log "Stopping existing stack (if any)..."
  "${COMPOSE_CMD[@]}" down --remove-orphans || true

  log "Starting offline VPS stack..."
  "${COMPOSE_CMD[@]}" up -d

  wait_for_garage_bootstrap
  wait_for_registry_health
  seed_registry_images
  wait_for_server_health
  seed_tokens
  seed_node_records
  configure_global_env

  log "Offline VPS stack is up."
}

main "$@"
