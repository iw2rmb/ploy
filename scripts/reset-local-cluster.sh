#!/usr/bin/env bash
set -euo pipefail

# reset-local-cluster.sh
# Hard reset for the local Docker-based Ploy stack:
# - Optionally runs tests (RUN_TESTS=1)
# - Tears down the local stack
# - Re-deploys a fresh local cluster via scripts/local-docker.sh
# - Drops/recreates the local ploy database during redeploy
#
# Usage:
#   ./scripts/reset-local-cluster.sh
#   RUN_TESTS=1 ./scripts/reset-local-cluster.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f local/docker-compose.yml}"
PLOY_LOCAL_PG_DSN="${PLOY_LOCAL_PG_DSN:-}"

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

main() {
  log "Checking prerequisites..."
  need docker
  need make

  if [[ -z "$PLOY_LOCAL_PG_DSN" ]]; then
    echo "error: PLOY_LOCAL_PG_DSN is required (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)" >&2
    exit 1
  fi
  export PLOY_LOCAL_PG_DSN

  if [[ "${RUN_TESTS:-}" == "1" ]]; then
    log "Running tests (make test)..."
    make test
  else
    log "RUN_TESTS not set to 1; skipping tests."
  fi

  log "Stopping local docker stack..."
  $COMPOSE_CMD down --remove-orphans

  log "Deploying fresh local cluster via scripts/local-docker.sh --drop-db..."
  ./scripts/local-docker.sh --drop-db

  log "Local cluster reset complete."
}

main "$@"
