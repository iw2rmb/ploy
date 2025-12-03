#!/usr/bin/env bash
set -euo pipefail

# reset-local-cluster.sh
# Hard reset for the local Docker-based Ploy stack:
# - Optionally runs tests (RUN_TESTS=1)
# - Tears down the local stack (drops DB by removing containers)
# - Re-deploys a fresh local cluster via scripts/deploy-locally.sh
# - Refreshes server/node binaries inside containers from local dist builds
#
# Usage:
#   ./scripts/reset-local-cluster.sh
#   RUN_TESTS=1 ./scripts/reset-local-cluster.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f local/docker-compose.yml}"

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
  need curl

  if [[ "${RUN_TESTS:-}" == "1" ]]; then
    log "Running tests (make test)..."
    make test
  else
    log "RUN_TESTS not set to 1; skipping tests."
  fi

  log "Stopping local docker stack (this also drops the ploy database)..."
  $COMPOSE_CMD down --remove-orphans

  log "Deploying fresh local cluster via scripts/deploy-locally.sh..."
  ./scripts/deploy-locally.sh

  # At this point, make build has already been run by deploy-locally.sh.
  # Ensure linux binaries exist for server/node override.
  if [[ ! -f "dist/ployd-linux" ]]; then
    echo "error: dist/ployd-linux not found. Run 'make build' first." >&2
    exit 1
  fi
  if [[ ! -f "dist/ployd-node-linux" ]]; then
    echo "error: dist/ployd-node-linux not found. Run 'make build' first." >&2
    exit 1
  fi

  log "Overriding server binary in local-server-1 with dist/ployd-linux..."
  docker cp dist/ployd-linux local-server-1:/usr/local/bin/ployd

  log "Overriding node binary in local-node-1 with dist/ployd-node-linux..."
  docker cp dist/ployd-node-linux local-node-1:/usr/local/bin/ployd-node

  log "Restarting server and node containers to pick up refreshed binaries..."
  docker restart local-server-1 local-node-1 >/dev/null

  log "Waiting for server health on http://localhost:8080/health..."
  for i in {1..60}; do
    if curl -fsS http://localhost:8080/health >/dev/null 2>&1; then
      log "Server is healthy."
      break
    fi
    sleep 1
    if [[ $i -eq 60 ]]; then
      echo "error: server did not become healthy in time" >&2
      exit 1
    fi
  done

  log "Local cluster reset complete. Binaries and database are fresh."
}

main "$@"

